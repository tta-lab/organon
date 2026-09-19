# Organon

Organon provides local CLI tools that give AI agents controlled access to
source editing, skills, project registries, token counts, and forge workflows.

## Language

**Derived clone path**:
The controlled local destination computed from a URL clone's host, owner, and
repository. Its owner and repository directory components are lowercase.
_Avoid_: remote path, checkout identity

**Repository identity**:
The canonical remote URL and its provider-specific owner and repository names.
It identifies the remote repository and is distinct from a derived clone path.
_Avoid_: clone path, local repository name

**Issue**:
A repository-scoped work item for describing and tracking a problem or proposed
change. An issue is distinct from a pull request and is identified within its repository.

**Issue body**:
The issue's primary description, separate from its title and discussion comments.
_Avoid_: issue content when it could include comments

**Issue comment**:
A discussion entry attached to an issue, separate from its primary body.
Adding a comment leaves the issue title and body unchanged.
