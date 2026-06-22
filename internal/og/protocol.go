package og

// Request is the typed local daemon request. CLI callers should only populate
// working-directory and operation fields; token fields are intentionally daemon-owned.
type Request struct {
	WorkDir string `json:"work_dir"`
	Force   bool   `json:"force,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Bump    string `json:"bump,omitempty"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body,omitempty"`
	Index   int64  `json:"index,omitempty"`
	State   string `json:"state,omitempty"`
	Tail    int    `json:"tail,omitempty"`

	Token    string `json:"token,omitempty"`
	TokenEnv string `json:"token_env,omitempty"`
}

// Response is the typed local daemon response.
type Response struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
	PR      *PullRequest `json:"pr,omitempty"`
	Lines   []string     `json:"lines,omitempty"`
}

// PullRequest is the stable PR shape returned to the CLI.
type PullRequest struct {
	Index        int64             `json:"index"`
	Number       int64             `json:"number,omitempty"`
	Title        string            `json:"title"`
	State        string            `json:"state"`
	Merged       bool              `json:"merged"`
	URL          string            `json:"url"`
	HTMLURL      string            `json:"html_url,omitempty"`
	Head         string            `json:"head"`
	Base         string            `json:"base"`
	Body         string            `json:"body"`
	SHA          string            `json:"head_sha,omitempty"`
	CI           *CIStatusResponse `json:"ci,omitempty"`
	CIFetchError string            `json:"ci_fetch_error,omitempty"`
}

// CIStatusResponse is the stable CI summary shape returned with PR JSON.
type CIStatusResponse struct {
	OK       bool       `json:"ok"`
	Error    string     `json:"error,omitempty"`
	State    string     `json:"state,omitempty"`
	Statuses []CIStatus `json:"statuses,omitempty"`
}

// CIStatus is a single CI check status.
type CIStatus struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
}

func success(resp Response) Response {
	resp.OK = true
	return resp
}

func DisplayPRURL(pr *PullRequest) string {
	if pr.HTMLURL != "" {
		return pr.HTMLURL
	}
	return pr.URL
}
