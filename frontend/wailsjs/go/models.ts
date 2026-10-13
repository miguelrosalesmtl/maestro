export namespace flow {
	
	export class Check {
	    name: string;
	    state: string;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	    }
	}
	export class Commit {
	    sha: string;
	    message: string;
	    author: string;
	    at: string;
	    adds: number;
	    dels: number;
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.message = source["message"];
	        this.author = source["author"];
	        this.at = source["at"];
	        this.adds = source["adds"];
	        this.dels = source["dels"];
	    }
	}
	export class Event {
	    id: string;
	    issueKey: string;
	    kind: string;
	    title: string;
	    detail: string;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.issueKey = source["issueKey"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.at = source["at"];
	    }
	}
	export class PullRequest {
	    number: number;
	    title: string;
	    url: string;
	    state: string;
	    checks: Check[];
	    approvals: number;
	    reviewers: string[];
	
	    static createFrom(source: any = {}) {
	        return new PullRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.state = source["state"];
	        this.checks = this.convertValues(source["checks"], Check);
	        this.approvals = source["approvals"];
	        this.reviewers = source["reviewers"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Stage {
	    id: string;
	    label: string;
	    state: string;
	    detail: string;
	    at: string;
	    trigger: string;
	
	    static createFrom(source: any = {}) {
	        return new Stage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.state = source["state"];
	        this.detail = source["detail"];
	        this.at = source["at"];
	        this.trigger = source["trigger"];
	    }
	}
	export class Flow {
	    issueKey: string;
	    repo: string;
	    branch: string;
	    started: boolean;
	    stages: Stage[];
	    commits: Commit[];
	    pr?: PullRequest;
	
	    static createFrom(source: any = {}) {
	        return new Flow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.issueKey = source["issueKey"];
	        this.repo = source["repo"];
	        this.branch = source["branch"];
	        this.started = source["started"];
	        this.stages = this.convertValues(source["stages"], Stage);
	        this.commits = this.convertValues(source["commits"], Commit);
	        this.pr = this.convertValues(source["pr"], PullRequest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Rule {
	    id: string;
	    trigger: string;
	    label: string;
	    toStatus: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.trigger = source["trigger"];
	        this.label = source["label"];
	        this.toStatus = source["toStatus"];
	        this.enabled = source["enabled"];
	    }
	}
	
	export class StepResult {
	    flow: Flow;
	    issue: tracker.Issue;
	    events: Event[];
	
	    static createFrom(source: any = {}) {
	        return new StepResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.flow = this.convertValues(source["flow"], Flow);
	        this.issue = this.convertValues(source["issue"], tracker.Issue);
	        this.events = this.convertValues(source["events"], Event);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class Connection {
	    mode: string;
	    state: string;
	    configured: boolean;
	    site: string;
	    deployment: string;
	    error?: string;
	    lastSync?: string;
	    issues: number;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.state = source["state"];
	        this.configured = source["configured"];
	        this.site = source["site"];
	        this.deployment = source["deployment"];
	        this.error = source["error"];
	        this.lastSync = source["lastSync"];
	        this.issues = source["issues"];
	    }
	}
	export class KeybindingsInfo {
	    overrides: Record<string, string>;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new KeybindingsInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overrides = source["overrides"];
	        this.path = source["path"];
	    }
	}
	export class LogInfo {
	    dir: string;
	    file: string;
	    previousCrash: string;
	
	    static createFrom(source: any = {}) {
	        return new LogInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.file = source["file"];
	        this.previousCrash = source["previousCrash"];
	    }
	}
	export class Snapshot {
	    me: tracker.User;
	    issues: tracker.Issue[];
	    rules: flow.Rule[];
	    activity: flow.Event[];
	    flows: flow.Flow[];
	    users: tracker.User[];
	    statuses: string[];
	    statusStates: Record<string, string>;
	    sprint: string;
	    childTypes: Record<string, Array<string>>;
	    linkTypes: Record<string, string>;
	    projects: string[];
	    live: boolean;
	    sprints: tracker.SprintInfo[];
	    versions: Record<string, Array<tracker.VersionInfo>>;
	    createDefaults: tracker.CreateDefaults;
	    timeTracking: boolean;
	    customColumns: tracker.CustomColumn[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.me = this.convertValues(source["me"], tracker.User);
	        this.issues = this.convertValues(source["issues"], tracker.Issue);
	        this.rules = this.convertValues(source["rules"], flow.Rule);
	        this.activity = this.convertValues(source["activity"], flow.Event);
	        this.flows = this.convertValues(source["flows"], flow.Flow);
	        this.users = this.convertValues(source["users"], tracker.User);
	        this.statuses = source["statuses"];
	        this.statusStates = source["statusStates"];
	        this.sprint = source["sprint"];
	        this.childTypes = source["childTypes"];
	        this.linkTypes = source["linkTypes"];
	        this.projects = source["projects"];
	        this.live = source["live"];
	        this.sprints = this.convertValues(source["sprints"], tracker.SprintInfo);
	        this.versions = this.convertValues(source["versions"], Array<tracker.VersionInfo>, true);
	        this.createDefaults = this.convertValues(source["createDefaults"], tracker.CreateDefaults);
	        this.timeTracking = source["timeTracking"];
	        this.customColumns = this.convertValues(source["customColumns"], tracker.CustomColumn);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace tracker {
	
	export class Attachment {
	    id: string;
	    name: string;
	    size: number;
	    mimeType: string;
	    created: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.mimeType = source["mimeType"];
	        this.created = source["created"];
	    }
	}
	export class User {
	    id: string;
	    name: string;
	    initials: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.initials = source["initials"];
	        this.color = source["color"];
	    }
	}
	export class Comment {
	    id: string;
	    author: User;
	    body: string;
	    created: string;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.author = this.convertValues(source["author"], User);
	        this.body = source["body"];
	        this.created = source["created"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateDefaults {
	    sprint: string;
	    fixVersion: string;
	    affectsVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sprint = source["sprint"];
	        this.fixVersion = source["fixVersion"];
	        this.affectsVersion = source["affectsVersion"];
	    }
	}
	export class CustomColumn {
	    id: string;
	    label: string;
	    role?: string;
	    show: string[];
	
	    static createFrom(source: any = {}) {
	        return new CustomColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.role = source["role"];
	        this.show = source["show"];
	    }
	}
	export class FieldPatch {
	    fixVersions?: string[];
	    affectsVersions?: string[];
	    dueDate?: string;
	    labels?: string[];
	    custom?: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new FieldPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fixVersions = source["fixVersions"];
	        this.affectsVersions = source["affectsVersions"];
	        this.dueDate = source["dueDate"];
	        this.labels = source["labels"];
	        this.custom = source["custom"];
	    }
	}
	export class FieldSpec {
	    id: string;
	    label: string;
	    kind: string;
	    required: boolean;
	    options?: string[];
	    role?: string;
	    default?: string;
	    requiredBy?: string;
	
	    static createFrom(source: any = {}) {
	        return new FieldSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.required = source["required"];
	        this.options = source["options"];
	        this.role = source["role"];
	        this.default = source["default"];
	        this.requiredBy = source["requiredBy"];
	    }
	}
	export class FieldValue {
	    id: string;
	    label: string;
	    role?: string;
	    kind: string;
	    text?: string;
	    items?: string[];
	    show: string[];
	    editable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FieldValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.role = source["role"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.items = source["items"];
	        this.show = source["show"];
	        this.editable = source["editable"];
	    }
	}
	export class TimeTracking {
	    original?: string;
	    remaining?: string;
	    spent?: string;
	
	    static createFrom(source: any = {}) {
	        return new TimeTracking(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original = source["original"];
	        this.remaining = source["remaining"];
	        this.spent = source["spent"];
	    }
	}
	export class Link {
	    type: string;
	    key: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.key = source["key"];
	    }
	}
	export class Issue {
	    key: string;
	    summary: string;
	    type: string;
	    status: string;
	    statusCategory: string;
	    state: string;
	    priority: string;
	    assignee?: User;
	    reporter: User;
	    labels: string[];
	    components: string[];
	    sprint: string;
	    storyPoints: number;
	    parentKey: string;
	    childKeys: string[];
	    links: Link[];
	    description: string;
	    comments: Comment[];
	    created: string;
	    updated: string;
	    url: string;
	    fixVersions: string[];
	    affectsVersions: string[];
	    dueDate?: string;
	    resolution?: string;
	    resolvedAt?: string;
	    environment?: string;
	    timeTracking?: TimeTracking;
	    security?: string;
	    attachments: Attachment[];
	    watchers: number;
	    watching: boolean;
	    votes: number;
	    creator?: User;
	    statusChangedAt?: string;
	    flagged: boolean;
	    carriedOver: number;
	    startDate?: string;
	    custom: FieldValue[];
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.summary = source["summary"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.statusCategory = source["statusCategory"];
	        this.state = source["state"];
	        this.priority = source["priority"];
	        this.assignee = this.convertValues(source["assignee"], User);
	        this.reporter = this.convertValues(source["reporter"], User);
	        this.labels = source["labels"];
	        this.components = source["components"];
	        this.sprint = source["sprint"];
	        this.storyPoints = source["storyPoints"];
	        this.parentKey = source["parentKey"];
	        this.childKeys = source["childKeys"];
	        this.links = this.convertValues(source["links"], Link);
	        this.description = source["description"];
	        this.comments = this.convertValues(source["comments"], Comment);
	        this.created = source["created"];
	        this.updated = source["updated"];
	        this.url = source["url"];
	        this.fixVersions = source["fixVersions"];
	        this.affectsVersions = source["affectsVersions"];
	        this.dueDate = source["dueDate"];
	        this.resolution = source["resolution"];
	        this.resolvedAt = source["resolvedAt"];
	        this.environment = source["environment"];
	        this.timeTracking = this.convertValues(source["timeTracking"], TimeTracking);
	        this.security = source["security"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.watchers = source["watchers"];
	        this.watching = source["watching"];
	        this.votes = source["votes"];
	        this.creator = this.convertValues(source["creator"], User);
	        this.statusChangedAt = source["statusChangedAt"];
	        this.flagged = source["flagged"];
	        this.carriedOver = source["carriedOver"];
	        this.startDate = source["startDate"];
	        this.custom = this.convertValues(source["custom"], FieldValue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NewIssue {
	    type: string;
	    summary: string;
	    description: string;
	    parentKey: string;
	    project: string;
	    priority: string;
	    assigneeId: string;
	    storyPoints: number;
	    labels: string[];
	    sprint: string;
	    fixVersions: string[];
	    affectsVersions: string[];
	    custom: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new NewIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.summary = source["summary"];
	        this.description = source["description"];
	        this.parentKey = source["parentKey"];
	        this.project = source["project"];
	        this.priority = source["priority"];
	        this.assigneeId = source["assigneeId"];
	        this.storyPoints = source["storyPoints"];
	        this.labels = source["labels"];
	        this.sprint = source["sprint"];
	        this.fixVersions = source["fixVersions"];
	        this.affectsVersions = source["affectsVersions"];
	        this.custom = source["custom"];
	    }
	}
	export class SprintInfo {
	    id: number;
	    name: string;
	    state: string;
	    goal?: string;
	    start?: string;
	    end?: string;
	    project: string;
	
	    static createFrom(source: any = {}) {
	        return new SprintInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.goal = source["goal"];
	        this.start = source["start"];
	        this.end = source["end"];
	        this.project = source["project"];
	    }
	}
	
	export class Transition {
	    id: string;
	    name: string;
	    to: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new Transition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.to = source["to"];
	        this.category = source["category"];
	    }
	}
	
	export class VersionInfo {
	    id: string;
	    name: string;
	    released: boolean;
	    releaseDate?: string;
	    overdue?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.released = source["released"];
	        this.releaseDate = source["releaseDate"];
	        this.overdue = source["overdue"];
	    }
	}

}

