package og

// Executor is the direct OG domain surface used by the CLI and MCP adapters.
// Implementations own project resolution, credentials, policy, Git, and forge behavior.
type Executor interface {
	GitPush(Request) (Response, error)
	GitPull(Request) (Response, error)
	GitTag(Request) (Response, error)
	GitClone(Request) (Response, error)
	PRCreate(Request) (Response, error)
	PRView(Request) (Response, error)
	PRFind(Request) (Response, error)
	PRGet(Request) (Response, error)
	PRModify(Request) (Response, error)
	PRComment(Request) (Response, error)
	PRChecks(Request) (Response, error)
	PRLog(Request) (Response, error)
	PRFailures(Request) (Response, error)
	AuthStatus(Request) (Response, error)
}
