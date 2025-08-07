package todo_utils

import "net/http"

type TodoApp struct {
	HttpClient *http.Client
	APIUrl     string
}

type CreateTaskRequest struct {
}

// TODO : implement this function
// json builder to build  json request to the backend server
func JsonBuilder() {

}

func InitTodoAPP(httpClient *http.Client, API_Url string) *TodoApp {
	return &TodoApp{
		HttpClient: httpClient,
		APIUrl:     API_Url,
	}
}

// TODO : implement these features
/*
	1. Create task
	2. Delete task
	3. Update task
	4. Read a task
	5. Read all task (paginated by a given number)
*/

func (t *TodoApp) CreateTask() {
	/*
		1. use the t.httpclient
		2. call the url with the correct postfix
		3. handle the response (formatting and such)
		4. return the formatted message
	*/
	t.HttpClient.Post()
}
