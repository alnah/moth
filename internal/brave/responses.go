package brave

type braveWebResponse struct {
	Web struct {
		Results []braveWebResult `json:"results"`
	} `json:"web"`
}

type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type braveImagesResponse struct {
	Results []braveImageResult `json:"results"`
}

type braveImageResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Thumbnail   struct {
		Src string `json:"src"`
	} `json:"thumbnail"`
	Properties struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"properties"`
}

type braveVideosResponse struct {
	Results []braveVideoResult `json:"results"`
}

type braveVideoResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Thumbnail   struct {
		Src string `json:"src"`
	} `json:"thumbnail"`
	Duration  string `json:"duration"`
	Publisher string `json:"publisher"`
}
