package x

type xPostListResponse struct {
	Data     []xPost       `json:"data"`
	Includes xIncludes     `json:"includes"`
	Meta     xResponseMeta `json:"meta"`
}

type xSinglePostResponse struct {
	Data     xPost         `json:"data"`
	Includes xIncludes     `json:"includes"`
	Meta     xResponseMeta `json:"meta"`
}

type xUserLookupResponse struct {
	Data xUser `json:"data"`
}

type xPost struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	AuthorID  string `json:"author_id"`
	CreatedAt string `json:"created_at"`
}

type xIncludes struct {
	Users []xUser `json:"users"`
}

type xUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type xResponseMeta struct {
	ResultCount int    `json:"result_count"`
	NextToken   string `json:"next_token"`
}
