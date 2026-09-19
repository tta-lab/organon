package main

import (
	"sync"

	"github.com/tta-lab/organon/internal/og"
)

// deferredExecutor keeps local MCP discovery available when forge configuration
// cannot be loaded. The first forge operation initializes the real executor;
// its executor or initialization error is retained for this MCP process.
type deferredExecutor struct {
	load func() (og.Executor, error)
}

func newDeferredExecutor(load func() (og.Executor, error)) deferredExecutor {
	return deferredExecutor{load: sync.OnceValues(load)}
}

func (e deferredExecutor) call(operation func(og.Executor) (og.Response, error)) (og.Response, error) {
	executor, err := e.load()
	if err != nil {
		return og.Response{}, err
	}
	return operation(executor)
}

func (e deferredExecutor) GitPush(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.GitPush(request) })
}
func (e deferredExecutor) GitPull(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.GitPull(request) })
}
func (e deferredExecutor) GitTag(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.GitTag(request) })
}
func (e deferredExecutor) GitClone(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.GitClone(request) })
}
func (e deferredExecutor) PRCreate(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRCreate(request) })
}
func (e deferredExecutor) PRView(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRView(request) })
}
func (e deferredExecutor) PRFind(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRFind(request) })
}
func (e deferredExecutor) PRGet(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRGet(request) })
}
func (e deferredExecutor) PRModify(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRModify(request) })
}
func (e deferredExecutor) PRComment(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRComment(request) })
}
func (e deferredExecutor) PRChecks(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRChecks(request) })
}
func (e deferredExecutor) PRLog(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRLog(request) })
}
func (e deferredExecutor) PRFailures(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRFailures(request) })
}
func (e deferredExecutor) PRMerge(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.PRMerge(request) })
}
func (e deferredExecutor) AuthStatus(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.AuthStatus(request) })
}
func (e deferredExecutor) IssueList(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueList(request) })
}
func (e deferredExecutor) IssueSearch(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueSearch(request) })
}
func (e deferredExecutor) IssueGet(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueGet(request) })
}
func (e deferredExecutor) IssueComments(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueComments(request) })
}
func (e deferredExecutor) IssueCreate(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueCreate(request) })
}
func (e deferredExecutor) IssueUpdateTitle(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueUpdateTitle(request) })
}
func (e deferredExecutor) IssueReplaceBody(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueReplaceBody(request) })
}
func (e deferredExecutor) IssueEditBody(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueEditBody(request) })
}
func (e deferredExecutor) IssueComment(request og.Request) (og.Response, error) {
	return e.call(func(executor og.Executor) (og.Response, error) { return executor.IssueComment(request) })
}
