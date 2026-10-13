# Maestro — Jira configuration data specification

Language: ASD-STE100 Simplified Technical English (Issue 9).
Status: Draft 1. Input for the configuration schema, the settings UI and the YAML import.

---

## 1. Purpose

This document gives all the data that Maestro must have to operate with a Jira site.

For each data item, this document tells you:

- The question that Maestro asks Jira (or the user).
- The Jira source of the answer.
- The function in Maestro that uses the data.
- If the data is mandatory or optional.
- The default value, if Maestro has one.
- The proposed key in the YAML file.

Use this document to:

1. Make the configuration schema.
2. Make the settings UI.
3. Make the YAML file that a person can import.

## 2. Scope

This document applies to:

- The Maestro desktop application (macOS and Linux).
- Jira Cloud. Section 15 gives the differences for Jira Data Center.
- The connection to Jira through an MCP server.

This document does not apply to:

- The design of the MCP server.
- The design of the user interface.

## 3. Terms

| Term | Meaning in this document |
|---|---|
| Site | One Jira instance, for example `https://your-org.atlassian.net`. |
| Project | One Jira project. A project has a key, for example `PLAT`. |
| Issue | One Jira item (initiative, epic, story, task, bug or sub-task). Maestro also calls an issue a "ticket". |
| Container issue | An issue that holds other issues: an initiative or an epic. A person does not do work directly on a container issue. |
| Work issue | An issue that a person does work on: a story, a task, a bug or a sub-task. |
| Hierarchy level | The position of an issue type in the parent–child structure. |
| Status | The Jira status of an issue, for example "In Review". |
| Status category | The Jira group of a status: `To Do`, `In Progress` or `Done`. |
| Maestro state | A fixed state that Maestro uses for its logic (section 7.3). |
| Transition | A Jira operation that moves an issue from one status to a different status. |
| Dev event | An event from the code side: branch created, commit pushed, pull request (PR) opened, checks passed or failed, review approved, PR merged. |
| Automation rule | A Maestro rule: "When dev event X occurs, move the issue to status Y." |
| Ticket terminal | A shell session in Maestro that is attached to one work issue. |
| Discovery | The procedure where Maestro asks Jira for its configuration (section 5). |
| Override | A value in the YAML file or in the settings UI that replaces a discovered value. |

## 4. General rules

1. Maestro must get each data item from Jira when Jira can supply it. This is "discovery".
2. The YAML file must only contain data that Jira cannot supply, or overrides.
3. If the YAML file and discovery give different values, Maestro must use the YAML value.
4. Maestro must keep the Jira ID and the Jira name of each item. Names can change. IDs do not change.
5. Maestro must use IDs in the YAML file where possible. A name is permitted as an alternative, for easy writing by hand.
6. If a mandatory data item is missing, Maestro must show the item in the settings UI. Maestro must not start the related function.
7. Maestro must not keep secrets (tokens, passwords) in the YAML file. Section 6 gives the procedure.

## 5. Discovery procedure

Maestro does these steps in this sequence. Each step tells you the Jira REST resource. The MCP server must supply equivalent tools.

| Step | Get | Jira Cloud REST resource |
|---|---|---|
| 1 | The current user | `GET /rest/api/3/myself` |
| 2 | The server type and version | `GET /rest/api/3/serverInfo` |
| 3 | The projects that the user can see | `GET /rest/api/3/project/search` |
| 4 | The issue types and the hierarchy levels | `GET /rest/api/3/issuetype`, `GET /rest/api/3/issuetype/project?projectId=…` |
| 5 | The initiative level (above epic) | Jira Premium plans: `GET /rest/api/3/issuetype` (field `hierarchyLevel`) |
| 6 | All fields, with custom field IDs | `GET /rest/api/3/field` |
| 7 | The fields for issue creation, for each project and issue type | `GET /rest/api/3/issue/createmeta/{projectIdOrKey}/issuetypes/{issueTypeId}` |
| 8 | The statuses and status categories | `GET /rest/api/3/statuses/search` |
| 9 | The workflows and transitions | `GET /rest/api/3/workflows/search` |
| 10 | The priorities | `GET /rest/api/3/priority/search` |
| 11 | The issue link types | `GET /rest/api/3/issueLinkType` |
| 12 | The assignable users, for each project | `GET /rest/api/3/user/assignable/search?project=…` |
| 13 | The permissions of the current user | `GET /rest/api/3/mypermissions?projectKey=…&permissions=…` |
| 14 | The boards, for each project | `GET /rest/agile/1.0/board?projectKeyOrId=…` |
| 15 | The board columns and their statuses | `GET /rest/agile/1.0/board/{boardId}/configuration` |
| 16 | The active sprint | `GET /rest/agile/1.0/board/{boardId}/sprint?state=active` |
| 17 | The development integration (if installed) | `GET /rest/dev-status/latest/issue/summary?issueId=…` |

After discovery, Maestro must:

1. Compare the result with the YAML overrides.
2. Show each conflict in the settings UI.
3. Keep the result in a local cache. The cache must have a time stamp.

## 6. Connection (CON)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| CON-01 | Site URL | What is the base URL of the Jira site? | User | All functions. "Open in browser" links. | Yes | — | `jira.site` |
| CON-02 | Deployment type | Is the site Jira Cloud or Jira Data Center? | `serverInfo.deploymentType` | Selection of API version and fields (section 15) | Yes | Discovered | `jira.deployment` |
| CON-03 | MCP server | Which MCP server does Maestro use? What is its command or URL? | User | All Jira calls | Yes | — | `jira.mcp.command` or `jira.mcp.url` |
| CON-04 | Credential reference | Where is the token? | User | MCP server start | Yes | — | `jira.mcp.credentialRef` |
| CON-05 | Current user | What is the account ID, display name and avatar of the user? | `myself` | "My work" view. Default assignee. Comments. | Yes | Discovered | — (do not put in YAML) |
| CON-06 | Time zone | Which time zone does the user use? | `myself.timeZone` | Dates in the Jira view | No | System time zone | `ui.timeZone` |
| CON-07 | Refresh interval | How frequently must Maestro get new data from Jira? | User | Synchronization | No | 60 seconds | `jira.sync.intervalSeconds` |
| CON-08 | Webhooks | Can Jira send events to Maestro? | Site administrator | Fast updates. If not available, Maestro uses CON-07. | No | Not available | `jira.sync.webhooks` |

Rules:

1. Put the token in the operating-system keychain (macOS Keychain, Linux Secret Service).
2. In the YAML file, write only a reference to the keychain item, for example `keychain:maestro/jira`.
3. Do not write the token in the YAML file.

## 7. Projects (PRJ)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| PRJ-01 | Projects in scope | Which projects must Maestro show? | User (list from `project/search`) | Sidebar tree. My work. Board. Roadmap. | Yes | — | `projects[].key` |
| PRJ-02 | Project ID and name | What is the ID and the name of each project? | `project/search` | Labels in the UI | Yes | Discovered | — |
| PRJ-03 | Project style | Is the project company-managed or team-managed? | `project.style` | Parent field selection (section 9, FLD-04) | Yes | Discovered | — |
| PRJ-04 | Default project for new epics | In which project does Maestro create an epic that has no parent? | User | Create dialog | No | The first project in PRJ-01 | `defaults.epicProject` |
| PRJ-05 | Initiative project | In which project are the initiatives? | User or discovery (issue type at level above epic) | Create dialog. Roadmap. | Only if initiatives are used | — | `hierarchy.initiativeProject` |

Note: The current mock uses the keys `INIT`, `PLAT` and `ONB`. These keys are examples only.

### 7.1 Repository mapping (per project)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| PRJ-10 | Git repositories | Which Git repositories contain the code for this project? | User | Code panel. PR links. Ticket terminal. | Yes, for the dev functions | — | `projects[].repos[].name` (for example `acme/platform`) |
| PRJ-11 | Local path | Where is each repository on this computer? | User | Working directory of the ticket terminal | Yes, for the ticket terminal | Home directory | `projects[].repos[].path` |
| PRJ-12 | Git host | Which Git host contains the repository? | User | PR and check data | Yes, for the dev functions | `github` | `projects[].repos[].host` |
| PRJ-13 | Base branch | Into which branch do PRs merge? | Git host | PR creation. "Merged" detection. | No | Default branch of the repository | `projects[].repos[].baseBranch` |
| PRJ-14 | Component mapping | If a project has many repositories: which component uses which repository? | User | Repository selection when work starts | No | First repository | `projects[].repos[].components[]` |

## 8. Issue types and hierarchy (HIE)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| HIE-01 | Issue types | Which issue types does each project use? What are their IDs, names and icons? | `issuetype/project` | Type icons. Create dialog. | Yes | Discovered | — |
| HIE-02 | Hierarchy level | What is the hierarchy level of each issue type? | `issuetype.hierarchyLevel` (−1 = sub-task, 0 = standard, 1 = epic, 2+ = levels above epic) | Parent–child rules | Yes | Discovered | — |
| HIE-03 | Maestro role | Which Maestro role does each issue type have? | User. Discovery gives a proposal. | Container or work logic. Icons. Colors. | Yes | Proposal from HIE-02 | `hierarchy.types[].role` (`initiative`, `epic`, `story`, `task`, `bug`, `subtask`) |
| HIE-04 | Allowed children | Which issue types can be the child of each issue type? | Discovery from HIE-02. User can limit it. | Create dialog. "+" button in the tree. Validation. | Yes | Discovered | `hierarchy.children` |
| HIE-05 | Work issue types | Which issue types are work issues? | Derived from HIE-03 | My work. Focus. Board. Ticket terminal. | Yes | Roles story, task, bug, sub-task | — |
| HIE-06 | Levels above initiative | Does the site have more levels (for example "Theme")? | `hierarchyLevel` ≥ 3 | Roadmap. Sidebar tree. | No | None | `hierarchy.types[]` |

Rules:

1. Maestro must not let a user create a child that HIE-04 does not permit.
2. A sub-task must have a parent.
3. A sub-task must use the sprint of its parent.

The current mock uses these rules (see `internal/tracker/models.go`, `ChildTypes`):

| Parent | Permitted children |
|---|---|
| Initiative | Epic |
| Epic | Story, Task, Bug |
| Story, Task, Bug | Sub-task |
| Sub-task | (none) |

## 9. Fields (FLD)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| FLD-01 | Summary | — (system field) | `summary` | All views | Yes | — | — |
| FLD-02 | Description format | Does the site use Atlassian Document Format (ADF) or wiki markup? | CON-02 (Cloud = ADF, Data Center = wiki) | Jira view. Create dialog. | Yes | Discovered | — |
| FLD-03 | Story points field | Which custom field contains the story points? | `field` (name "Story point estimate" or "Story Points") | Board totals. My work totals. Create dialog. | No | Discovered by name | `fields.storyPoints` (for example `customfield_10016`) |
| FLD-04 | Parent field | Which field connects a child to its parent? | PRJ-03: team-managed = `parent`. Company-managed: `parent` (new) or "Epic Link" custom field (old). | Hierarchy. Create dialog. | Yes | Discovered | `fields.parent` |
| FLD-05 | Initiative parent field | Which field connects an epic to its initiative? | `parent` (Premium plans) or "Parent Link" custom field | Roadmap. Create dialog. | Only if initiatives are used | Discovered | `fields.initiativeParent` |
| FLD-06 | Sprint field | Which custom field contains the sprint? | `field` (schema `com.pyxis.greenhopper.jira:gh-sprint`) | Board. Create dialog ("Add to sprint"). | Yes, for the board | Discovered | `fields.sprint` |
| FLD-07 | Priorities | Which priorities does the site use? What is their order, icon and color? | `priority/search` | Priority icons. Sort order in My work. Create dialog. | Yes | Discovered | `fields.priorities[]` (order only) |
| FLD-08 | Default priority | Which priority does a new issue get? | `priority.isDefault` | Create dialog | No | Discovered | `defaults.priority` |
| FLD-09 | Labels | — (system field) | `labels` | Chips. Create dialog. | No | — | — |
| FLD-10 | Components | Which components does each project have? | `project/{key}/components` | Jira view. PRJ-14. | No | Discovered | — |
| FLD-11 | Required fields at creation | Which fields are mandatory when a user creates each issue type? | `createmeta` (`required: true`) | Create dialog. Validation. | Yes | Discovered | — |
| FLD-12 | Default values at creation | Which values must Maestro set for the user? | User | Create dialog | No | Assignee = current user. Sprint = active sprint (work issues only). | `defaults.create` |
| FLD-13 | Fields in the Jira view | Which fields must the Details panel show, and in which order? | User | Jira view | No | Assignee, Reporter, Development, Labels, Parent, Sprint, Story points, Priority, Components, Issue type | `ui.jiraView.fields[]` |
| FLD-14 | Team field | Does the site use a "Team" field? Which field ID? | `field` | Filter in My work (future) | No | None | `fields.team` |

## 10. Workflow, statuses and transitions (WFL)

### 10.1 Data items

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| WFL-01 | Statuses | Which statuses does each workflow use? What are their IDs and names? | `statuses/search` | Status pills. Board columns. Flow diagram. | Yes | Discovered | — |
| WFL-02 | Status category | What is the category of each status? | `status.statusCategory` | Colors. "Done" detection. Progress. | Yes | Discovered | — |
| WFL-03 | Status order | In which order must Maestro show the statuses? | Board columns (BRD-02). User can change it. | Board. Status menu. Shortcut "next status" (`l`) and "previous status" (`h`). | Yes | Board column order | `workflow.order[]` |
| WFL-04 | Transitions | From each status, which transitions are available? What are their IDs and target statuses? | `workflows/search`, or `GET /rest/api/3/issue/{key}/transitions` for each issue | Status menu. Drag and drop on the board. Automation rules. | Yes | Discovered | — |
| WFL-05 | Transition screens | Does a transition show a screen with mandatory fields (for example "Resolution")? | `GET /issue/{key}/transitions?expand=transitions.fields` | Status change. Maestro must ask for the field values. | Yes | Discovered | — |
| WFL-06 | Resolution | Which resolution does Maestro set when it moves an issue to a "Done" status? | User | Status change | Only if WFL-05 requires it | "Done" | `workflow.defaultResolution` |
| WFL-07 | Workflow per issue type | Does each issue type have a different workflow? | Workflow scheme | Status menu for each issue | Yes | Discovered | — |

### 10.2 Maestro states

Maestro uses five fixed states for its logic. Each Jira status must map to one Maestro state.

| Maestro state | Meaning | Color | Example Jira statuses |
|---|---|---|---|
| `todo` | Work is not started. | Gray | To Do, Backlog, Selected for Development |
| `inProgress` | Work is started. | Blue | In Progress, In Development |
| `inReview` | Code is in review. | Violet | In Review, Code Review, QA |
| `blocked` | Work cannot continue. | Red | Blocked, On Hold |
| `done` | Work is complete. | Green | Done, Closed, Released |

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| WFL-10 | State mapping | Which Maestro state does each Jira status have? | Proposal from WFL-02. User must confirm `inReview` and `blocked`. | Colors. Flow diagram. My work groups. Automation rules. | Yes | `To Do` category → `todo`. `Done` category → `done`. `In Progress` category → `inProgress`. | `workflow.states` |
| WFL-11 | Flow diagram statuses | Which statuses does the "Ticket ⇄ Code" diagram show, and above which dev event? | Derived from WFL-10 and AUT-01 | Focus view | No | One status for each of `todo`, `inProgress`, `inReview`, `done` | `ui.flowDiagram.statuses[]` |

Rules:

1. The Jira category "In Progress" contains both `inProgress` and `inReview` statuses. Discovery cannot make this difference. The user must confirm it.
2. If no status maps to `blocked`, Maestro must not show the "Blocked" choice.
3. If a project has more than one status for one Maestro state, the automation rules must name the exact status (AUT-02).

## 11. Issue links (LNK)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| LNK-01 | Link types | Which link types does the site have? What are the ID, the outward name and the inward name of each? | `issueLinkType` | Jira view "Linked issues". Link form. | Yes | Discovered | — |
| LNK-02 | Blocking link | Which link type means "blocks / is blocked by"? | User. Discovery gives a proposal (name "Blocks"). | "Blocked by" warning in My work and Focus. | No | Link type with the name "Blocks" | `links.blocking` |
| LNK-03 | Visible link types | Which link types must the link form show? | User | Link form | No | All | `links.visible[]` |

The current mock uses these link names: blocks / is blocked by, relates to, duplicates / is duplicated by, clones / is cloned by.

## 12. Agile boards and sprints (BRD)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| BRD-01 | Board | Which board does Maestro use for each project? | `agile/board` | Board view. Sprint. | Yes, for the board | The first Scrum board of the project | `projects[].board` |
| BRD-02 | Board columns | Which columns does the board have? Which statuses does each column contain? | `board/{id}/configuration` | Board columns. WFL-03. | Yes | Discovered | — |
| BRD-03 | Board type | Is the board Scrum or Kanban? | `board.type` | Sprint functions. A Kanban board has no sprint. | Yes | Discovered | — |
| BRD-04 | Active sprint | What is the active sprint? What are its ID, name, start date and end date? | `board/{id}/sprint?state=active` | Board title. "Ends in N days". Sprint filter in My work. Create dialog. | Yes, for Scrum | Discovered | — |
| BRD-05 | Sub-tasks on the board | Must the board show sub-tasks as cards? | User | Board | No | No (Maestro shows a counter on the parent card) | `ui.board.showSubtasks` |
| BRD-06 | Board filter | Which issues does the board contain? | `board/{id}/filter` | Board | Yes | Discovered | — |

## 13. Users and permissions (USR)

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| USR-01 | Assignable users | Which users can Maestro assign to issues in each project? | `user/assignable/search` | Create dialog. Person filter in My work. | Yes | Discovered | — |
| USR-02 | Team members | Which users must the person filter in My work show first? | User | My work | No | All users from USR-01 | `team.members[]` (account IDs) |
| USR-03 | Avatars and colors | Which avatar does each user have? | `user.avatarUrls` | Avatars | No | Initials with a generated color | — |
| USR-04 | Permissions | Can the current user create, edit, transition, link, assign and comment in each project? | `mypermissions` (`CREATE_ISSUES`, `EDIT_ISSUES`, `TRANSITION_ISSUES`, `LINK_ISSUES`, `ASSIGN_ISSUES`, `ADD_COMMENTS`) | Maestro must hide or disable the related controls. | Yes | Discovered | — |

## 14. Development integration and automation (DEV, AUT)

### 14.1 Branch and PR conventions

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| DEV-01 | Branch name template | Which name does Maestro give to a new branch? | User | "Start work". Ticket terminal command "create branch". | Yes | `{prefix}/{key}-{slug}` | `dev.branch.template` |
| DEV-02 | Branch prefix per issue type | Which prefix does each issue type use? | User | DEV-01 | No | Bug → `fix`. Other types → `feature`. | `dev.branch.prefix` |
| DEV-03 | Slug length | How many words of the summary go into the branch name? | User | DEV-01 | No | 5 | `dev.branch.slugWords` |
| DEV-04 | PR title template | Which title does Maestro give to a new PR? | User | Ticket terminal command "open PR" | No | `[{key}] {summary}` | `dev.pr.titleTemplate` |
| DEV-05 | PR body template | Which text does Maestro put in a new PR? | User | Ticket terminal command "open PR" | No | `Jira: {url}` | `dev.pr.bodyTemplate` |
| DEV-06 | Merge method | Which merge method does Maestro use? | User or repository settings | Ticket terminal command "merge PR" | No | `squash` | `dev.pr.mergeMethod` |
| DEV-07 | Required checks | Which CI checks must pass before review? | Git host (branch protection) | "Checks" stage in the flow diagram | No | All checks on the PR | `dev.checks.required[]` |
| DEV-08 | Required approvals | How many approvals must a PR have? | Git host (branch protection) | "Review" stage. "n/m approvals". | No | 1 | `dev.review.requiredApprovals` |
| DEV-09 | Issue key detection | How does Maestro find the issue key in a branch, a commit or a PR? | User | Connection of dev events to issues | No | Regular expression `[A-Z][A-Z0-9]+-\d+` | `dev.keyPattern` |
| DEV-10 | Jira development panel | Is a Git integration for Jira installed (for example "GitHub for Jira")? | `dev-status` | Development section in the Jira view | No | Not installed (Maestro uses its own data) | — |

### 14.2 Automation rules

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| AUT-01 | Rules | For each dev event, to which status must Maestro move the issue? | User | Automation engine. Flow diagram. | Yes | Table below | `automation.rules[]` |
| AUT-02 | Target status per project | Does the target status change for each project or issue type? | User | Automation engine | No | Same for all projects | `automation.rules[].projects[]`, `automation.rules[].issueTypes[]` |
| AUT-03 | Jira automation conflicts | Does Jira already move issues on these events (Jira Automation or the Git integration)? | Site administrator | Maestro must disable its rule for that event. Two systems must not move the same issue. | Yes | Unknown — ask the user | `automation.rules[].enabled` |
| AUT-04 | Parent roll-up | When all sub-tasks are done, must Maestro move the parent? | User | Automation engine (future) | No | No | `automation.rollUp.subtasksToParent` |

Default rules (current mock, `internal/flow/flow.go`):

| Dev event (`trigger`) | Target Maestro state | Enabled |
|---|---|---|
| `branch_created` | `inProgress` | Yes |
| `pr_opened` | `inReview` | Yes |
| `changes_requested` | `inProgress` | Yes |
| `pr_merged` | `done` | Yes |
| `ci_failed` | `blocked` | No |

Rules:

1. In the YAML file, a rule can name a Maestro state or an exact Jira status.
2. If a rule names a Maestro state, Maestro must use WFL-10 to find the Jira status.
3. Maestro must use a transition from WFL-04. If no transition goes to the target status, Maestro must not move the issue. Maestro must show a warning.
4. Maestro must not move an issue to a status that it already has.

### 14.3 Ticket terminal

| ID | Data | Question | Source | Used by | Mandatory | Default | YAML key |
|---|---|---|---|---|---|---|---|
| TRM-01 | Shell | Which shell must the ticket terminal start? | User | Ticket terminal | No | `$SHELL`, as a login shell | `terminal.shell` |
| TRM-02 | Working directory | In which directory must the terminal start? | PRJ-11 | Ticket terminal | No | Repository path, else home directory | — |
| TRM-03 | Environment variables | Which variables must Maestro set? | User | Ticket terminal | No | `MAESTRO_TICKET`, `MAESTRO_BRANCH`, `MAESTRO_SUMMARY` | `terminal.env` |
| TRM-04 | Quick commands | Which commands must the terminal bar show? | User | Ticket terminal bar | No | create branch, checkout, commit, push, open PR, merge PR | `terminal.commands[]` |
| TRM-05 | Watched commands | Which typed commands are dev events? | User | Automation engine | No | `git checkout -b`, `git switch -c`, `git push`, `gh pr create`, `gh pr merge` | `terminal.watch[]` |

## 15. Differences for Jira Data Center

| Item | Jira Cloud | Jira Data Center |
|---|---|---|
| API version | `/rest/api/3` | `/rest/api/2` |
| Description format (FLD-02) | ADF (JSON) | Wiki markup |
| User ID | `accountId` | `name` or `key` |
| Parent of an epic child (FLD-04) | `parent` | "Epic Link" custom field |
| Initiative level (HIE-06) | Premium plans | Advanced Roadmaps app |
| Workflow search (WFL-04) | `workflows/search` | Use `issue/{key}/transitions` for each issue |

## 16. Local preferences (not Jira data)

These items are not Jira configuration. Maestro keeps them in the user configuration directory. The YAML import must accept them in a separate section.

| ID | Data | Used by | Default | Location now | YAML key |
|---|---|---|---|---|---|
| LOC-01 | Keyboard shortcuts | All views. `?` sheet. | Built-in list (`frontend/src/keys.tsx`) | `<config dir>/maestro/keybindings.json` | `ui.keybindings` |
| LOC-02 | Default view | Start of the application | My work | — | `ui.defaultView` |
| LOC-03 | Terminal panel height and state | Ticket terminal | Open, 280 px | — | `ui.terminal` |

## 17. Current mock assumptions to remove

The mock contains fixed values. Each fixed value must come from the configuration.

| Fixed value | File | Replace with |
|---|---|---|
| Site URL `https://your-org.atlassian.net/browse/` | `internal/tracker/mock.go` | CON-01 |
| Sprint name "Sprint 42" | `internal/tracker/mock.go` | BRD-04 |
| Project keys `INIT`, `PLAT`, `ONB` | `internal/tracker/mock.go`, `frontend/src/components/CreateIssueModal.tsx` | PRJ-01, PRJ-04, PRJ-05 |
| Status list (To Do, In Progress, In Review, Blocked, Done) | `internal/tracker/models.go` | WFL-01, WFL-03 |
| Status colors by status name | `frontend/src/lib/util.ts` (`statusTone`) | WFL-10 |
| Flow diagram positions by status name | `frontend/src/components/FlowBridge.tsx` | WFL-11 |
| Hierarchy rules | `internal/tracker/models.go` (`ChildTypes`) | HIE-04 |
| Link type names | `internal/tracker/models.go` (`LinkTypes`) | LNK-01 |
| Priority list | `frontend/src/components/CreateIssueModal.tsx`, `bits.tsx` | FLD-07 |
| Repository `acme/platform` | `internal/flow/flow.go` | PRJ-10 |
| Branch prefixes `feature` and `fix` | `internal/flow/flow.go`, `frontend/src/lib/util.ts` | DEV-01, DEV-02 |
| Checks `build`, `unit-tests`, `lint` and two reviewers | `internal/flow/flow.go` | DEV-07, DEV-08 |
| Default automation rules | `internal/flow/flow.go` | AUT-01 |
| "Ends in 6 days" | `frontend/src/components/BoardView.tsx` | BRD-04 |

## 18. Validation of the YAML file

Maestro must do these checks when a user imports a YAML file:

1. Make sure that the file has a `version` key. Reject the file if the version is not known.
2. Make sure that each project key in `projects[]` exists on the site.
3. Make sure that each status name or ID exists in the workflow of the project.
4. Make sure that each Jira status maps to exactly one Maestro state.
5. Make sure that each automation rule names a status that a transition can reach.
6. Make sure that each custom field ID exists and has the correct type.
7. Make sure that each local path in `repos[].path` exists on this computer. If a path does not exist, show a warning. Do not reject the file.
8. Make sure that the file does not contain a token or a password.
9. Show all errors together. Do not stop at the first error.
10. Do not apply a part of the file. Apply all of it or none of it.

## Appendix A. Draft YAML structure

This structure is a draft. It shows the proposed keys only. The schema will give the final structure.

```yaml
version: 1

jira:
  site: https://your-org.atlassian.net          # CON-01
  deployment: cloud                              # CON-02
  mcp:
    command: ["npx", "-y", "@atlassian/mcp-server"]   # CON-03 (example)
    credentialRef: keychain:maestro/jira         # CON-04
  sync:
    intervalSeconds: 60                          # CON-07

projects:                                        # PRJ-01
  - key: PLAT
    board: 42                                    # BRD-01
    repos:                                       # PRJ-10..14
      - name: acme/platform
        host: github
        path: ~/code/platform
        baseBranch: main

hierarchy:
  initiativeProject: INIT                        # PRJ-05
  types:                                         # HIE-03
    - { id: "10000", role: epic }
    - { id: "10001", role: story }
    - { id: "10004", role: bug }
    - { id: "10003", role: subtask }

fields:
  storyPoints: customfield_10016                 # FLD-03
  sprint: customfield_10020                      # FLD-06

workflow:
  states:                                        # WFL-10
    "To Do": todo
    "In Progress": inProgress
    "In Review": inReview
    "Blocked": blocked
    "Done": done
  defaultResolution: Done                        # WFL-06

links:
  blocking: Blocks                               # LNK-02

dev:
  branch:
    template: "{prefix}/{key}-{slug}"            # DEV-01
    prefix: { bug: fix, default: feature }       # DEV-02
  pr:
    titleTemplate: "[{key}] {summary}"           # DEV-04
    mergeMethod: squash                          # DEV-06

automation:
  rules:                                         # AUT-01
    - { trigger: branch_created, to: inProgress, enabled: true }
    - { trigger: pr_opened,      to: inReview,   enabled: true }
    - { trigger: pr_merged,      to: done,       enabled: true }
    - { trigger: ci_failed,      to: blocked,    enabled: false }

ui:
  defaultView: mine                              # LOC-02
  keybindings:                                   # LOC-01
    nav.board: "g b"
```

## Appendix B. Open questions for the team

1. Do we support more than one Jira site in one Maestro installation?
2. Is Jira Data Center in scope for version 1?
3. Does the organization use Jira Automation for the same transitions (AUT-03)?
4. Which MCP server do we use? Does it supply all the tools in section 5?
5. Do we share one YAML file for the team and keep a second file for personal overrides?
