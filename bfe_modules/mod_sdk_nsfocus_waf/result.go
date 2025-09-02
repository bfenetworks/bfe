package mod_sdk_nsfocus_waf

type NsfocusWafAction uint

const (
	WAF_RESULT_PASS = iota
	WAF_RESULT_BLOCK
)

type NsfocusWafResult struct {
	LogID  string
	Action int
}

// GetResultFlag get result action
func (r *NsfocusWafResult) GetResultFlag() int {
	return r.Action
}

// GetEventId get attack event id
func (r *NsfocusWafResult) GetEventId() string {
	return r.LogID
}
