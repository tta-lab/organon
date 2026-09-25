# Organon

Organon provides local CLI tools that give AI agents controlled access to
source editing, skills, project registries, token counts, and forge workflows.

## Language

**Registered project**:
A repository identified by a canonical remote, with a configured alias and local
checkout path. A standalone local directory without a canonical remote is not a
registered project.
_Avoid_: arbitrary workspace, directory bookmark

**Derived clone path**:
The controlled local destination computed from a URL clone's host, owner, and
repository. Its owner and repository directory components are lowercase.
_Avoid_: remote path, checkout identity

**Repository identity**:
The canonical remote URL and its provider-specific owner and repository names.
It identifies the remote repository and is distinct from a derived clone path.
_Avoid_: clone path, local repository name

**Working diff**:
The tracked-file change set from the current branch's merge base with the
local origin default branch through the current working tree. It includes committed,
staged, and unstaged changes; untracked paths are reported separately.
_Avoid_: current diff, branch diff, PR diff

**Issue**:
A repository-scoped work item for describing and tracking a problem or proposed
change. An issue is distinct from a pull request and is identified within its repository.

**Issue body**:
The issue's primary description, separate from its title and discussion comments.
_Avoid_: issue content when it could include comments

**Issue comment**:
A discussion entry attached to an issue, separate from its primary body.
Adding a comment leaves the issue title and body unchanged.
