package JiraAccess

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	jira "gopkg.in/andygrunwald/go-jira.v1"
)

type Jira_t struct {
	client *jira.Client
	auth   jira.BasicAuthTransport
	jiraDB string
}

func GetJiraDriver(user, pw, url string) Jira_t {
	jira := Jira_t{
		jiraDB: url,
		auth: jira.BasicAuthTransport{
			Username: user,
			Password: pw,
		},
	}
	return jira
}

func checkClient(j *Jira_t) error {
	if j.client == nil {
		if err := j.newClient(); err != nil {
			return fmt.Errorf("Can not create Jira Client\n" + err.Error())
		}
	}

	return nil
}

func (j *Jira_t) newClient() error {
	var err error
	j.client, err = jira.NewClient(j.auth.Client(), j.jiraDB)
	return err
}

func (j *Jira_t) GetFieldRawData(name string) ([]byte, error) {
	var err error
	if err := checkClient(j); err != nil {
		return nil, err
	}

	var issue *jira.Issue
	if issue, _, err = j.client.Issue.Get(name, nil); err != nil {
		return nil, err
	}

	var data []byte
	if data, err = issue.Fields.MarshalJSON(); err != nil {
		return nil, err
	}

	return data, nil
}

func (j *Jira_t) GetJiraId(name string) (string, error) {
	var err error
	if err := checkClient(j); err != nil {
		return "", err
	}

	var issue *jira.Issue
	if issue, _, err = j.client.Issue.Get(name, nil); err != nil {
		return "", err
	}

	return issue.ID, nil
}

func (j *Jira_t) UnmarshalJson(issue string, v interface{}) (err error) {
	if err = checkClient(j); err != nil {
		return err
	}

	var i *jira.Issue
	if i, _, err = j.client.Issue.Get(issue, nil); err != nil {
		return
	}

	var data []byte
	if data, err = i.Fields.MarshalJSON(); err != nil {
		return
	}

	return json.Unmarshal(data, &v)
}

func (j *Jira_t) GetSummary(issue string) (string, error) {
	var err error
	if err = checkClient(j); err != nil {
		return "", err
	}

	var i *jira.Issue
	if i, _, err = j.client.Issue.Get(issue, nil); err != nil {
		return "", err
	}

	return i.Fields.Summary, nil
}

func (j *Jira_t) Search(jql string, threadcount int) ([]string, error) {
	var err error
	if err = checkClient(j); err != nil {
		return nil, err
	}

	option := jira.SearchOptions{
		MaxResults: 25,
		Fields:     []string{},
	}

	var list []jira.Issue
	issues := []string{}

	listCh := make(chan []jira.Issue, 64)
	done := make(chan interface{})
	go func() {
		total := 0
		for {
			select {
			case l := <-listCh:
				list = append(list, l...)
				total += len(l)
				fmt.Printf("Scan %d issues\r", total)
			case <-done:
				return
			}
		}
	}()

	optionCh := make(chan jira.SearchOptions, threadcount)
	doneCh := make(chan int)
	wg := sync.WaitGroup{}
	for t := 0; t < threadcount; t += 1 {
		wg.Add(1)
		go func(t int) {
			defer wg.Done()
			for {
				select {
				case o := <-optionCh:
					localList, _, _ := j.client.Issue.Search(jql, &o)
					if len(localList) == 0 {
						doneCh <- 1
						return
					} else {
						doneCh <- 0
					}
					listCh <- localList
				}
			}
		}(t)
	}

	for t := 0; t < threadcount; t += 1 {
		optionCh <- option
		option.StartAt += option.MaxResults
	}

	totalDone := 0
LOOP:
	for {
		select {
		case r := <-doneCh:
			totalDone += r
			if totalDone == threadcount {
				break LOOP
			} else {
				optionCh <- option
				option.StartAt += option.MaxResults
			}
		}
	}

	wg.Wait()
	close(done)
	close(doneCh)

	for _, v := range list {
		issues = append(issues, v.Key)
	}

	return issues, nil
}

func (j *Jira_t) Scan(project string, minutes int, threadcount int) ([]string, error) {
	var err error
	if err = checkClient(j); err != nil {
		return nil, err
	}

	if threadcount <= 0 {
		return nil, fmt.Errorf("Invalid thread count")
	}

	jqlProject := "project=" + project

	jql := jqlProject
	if minutes < 0 {
		jqlDate := "updatedDate >= " + strconv.Itoa(minutes) + "m"
		jql += " AND " + jqlDate
	}

	return j.Search(jql, threadcount)
}

// Example of jira.Issue to create JIRA

// customField := map[string]interface{}{
// 	"customfield_10066": "SSDFWVEGA-861", //> epic key
// 	"customfield_12054": map[string]string{ //> sub type
// 		"value": "Design",
// 	},
// 	"customfield_10065": 5016, //> sprint id
// 	"customfield_10062": 1,    //> story point
// }

// issue := jira.Issue{
// 	Fields: &jira.IssueFields{
// 		Summary:     "[Protocol] Test Create JIRA",
// 		Description: time.Now().Local().Format(time.RFC3339),
// 		Project: jira.Project{
// 			Key: "SSDFWVEGA",
// 		},
// 		Assignee: &jira.User{
// 			Name: "afei",
// 		},
// 		Type: jira.IssueType{
// 			Name: "Story",
// 		},
// 		FixVersions: []*jira.FixVersion{
// 			{
// 				Name: "FDR_v2",
// 			},
// 		},
// 		Components: []*jira.Component{
// 			{
// 				Name: "Protocol_Team",
// 			},
// 		},
// 		Unknowns: customField,
// 	},
// }

// End of example

func (j *Jira_t) CreateJira(issue *jira.Issue) (Key string, err error) {
	if err := checkClient(j); err != nil {
		return "", err
	}

	if i, resp, err := j.client.Issue.Create(issue); err != nil {
		var v interface{}

		json.NewDecoder(resp.Body).Decode(&v)
		result, _ := json.MarshalIndent(v, "  ", "")
		return "", fmt.Errorf("Create Jira fail\n" + err.Error() + "\n" + string(result))
	} else {
		return i.Key, nil
	}
}

func (j *Jira_t) UpdateJira(issue *jira.Issue) (Key string, err error) {
	if err := checkClient(j); err != nil {
		return "", err
	}

	if i, resp, err := j.client.Issue.Update(issue); err != nil {
		var v interface{}

		json.NewDecoder(resp.Body).Decode(&v)
		result, _ := json.MarshalIndent(v, "  ", "")
		return "", fmt.Errorf("Create Jira fail\n" + err.Error() + "\n" + string(result))
	} else {
		return i.Key, nil
	}
}
