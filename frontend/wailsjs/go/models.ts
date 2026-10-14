export namespace config {
	
	export class AgentMCP {
	    Register?: boolean;
	    Server: string;
	    URL: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentMCP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Register = source["Register"];
	        this.Server = source["Server"];
	        this.URL = source["URL"];
	    }
	}
	export class Agent {
	    MCP: AgentMCP;
	    ShareTicketContent: boolean;
	    ShareSecuredIssues: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.MCP = this.convertValues(source["MCP"], AgentMCP);
	        this.ShareTicketContent = source["ShareTicketContent"];
	        this.ShareSecuredIssues = source["ShareSecuredIssues"];
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
	
	export class Auth {
	    Method: string;
	    Email: string;
	    CredentialRef: string;
	    TokenExpires: string;
	
	    static createFrom(source: any = {}) {
	        return new Auth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Method = source["Method"];
	        this.Email = source["Email"];
	        this.CredentialRef = source["CredentialRef"];
	        this.TokenExpires = source["TokenExpires"];
	    }
	}
	export class RollUp {
	    SubtasksToParent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RollUp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SubtasksToParent = source["SubtasksToParent"];
	    }
	}
	export class Rule {
	    Trigger: string;
	    To: string;
	    Enabled?: boolean;
	    Projects: string[];
	    IssueTypes: string[];
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Trigger = source["Trigger"];
	        this.To = source["To"];
	        this.Enabled = source["Enabled"];
	        this.Projects = source["Projects"];
	        this.IssueTypes = source["IssueTypes"];
	    }
	}
	export class Automation {
	    Rules: Rule[];
	    AssignOnStart?: boolean;
	    RollUp: RollUp;
	    AnyAssignee: boolean;
	    AnyAuthor: boolean;
	    DryRun: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Automation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Rules = this.convertValues(source["Rules"], Rule);
	        this.AssignOnStart = source["AssignOnStart"];
	        this.RollUp = this.convertValues(source["RollUp"], RollUp);
	        this.AnyAssignee = source["AnyAssignee"];
	        this.AnyAuthor = source["AnyAuthor"];
	        this.DryRun = source["DryRun"];
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
	export class Branch {
	    Template: string;
	    Prefix: Record<string, string>;
	    SlugWords: number;
	
	    static createFrom(source: any = {}) {
	        return new Branch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Template = source["Template"];
	        this.Prefix = source["Prefix"];
	        this.SlugWords = source["SlugWords"];
	    }
	}
	export class Checks {
	    Required: string[];
	
	    static createFrom(source: any = {}) {
	        return new Checks(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Required = source["Required"];
	    }
	}
	export class FromBranch {
	    Enabled: boolean;
	    Template: string;
	
	    static createFrom(source: any = {}) {
	        return new FromBranch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.Template = source["Template"];
	    }
	}
	export class Versions {
	    Pattern: string;
	    RequireFixVersionOnDone: boolean;
	    FromBranch: FromBranch;
	    AllowManage: boolean;
	    ReleaseNotesTemplate: string;
	
	    static createFrom(source: any = {}) {
	        return new Versions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Pattern = source["Pattern"];
	        this.RequireFixVersionOnDone = source["RequireFixVersionOnDone"];
	        this.FromBranch = this.convertValues(source["FromBranch"], FromBranch);
	        this.AllowManage = source["AllowManage"];
	        this.ReleaseNotesTemplate = source["ReleaseNotesTemplate"];
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
	export class Sprints {
	    ClosedHistory: number;
	    VelocitySprints: number;
	    AllowManage: boolean;
	    NameTemplate: string;
	
	    static createFrom(source: any = {}) {
	        return new Sprints(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ClosedHistory = source["ClosedHistory"];
	        this.VelocitySprints = source["VelocitySprints"];
	        this.AllowManage = source["AllowManage"];
	        this.NameTemplate = source["NameTemplate"];
	    }
	}
	export class Log {
	    Level: string;
	
	    static createFrom(source: any = {}) {
	        return new Log(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Level = source["Level"];
	    }
	}
	export class UIJiraView {
	    Fields: string[];
	
	    static createFrom(source: any = {}) {
	        return new UIJiraView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Fields = source["Fields"];
	    }
	}
	export class UIBoard {
	    ShowSubtasks: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UIBoard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ShowSubtasks = source["ShowSubtasks"];
	    }
	}
	export class UI {
	    DefaultView: string;
	    TimeZone: string;
	    Keybindings: Record<string, string>;
	    Board: UIBoard;
	    JiraView: UIJiraView;
	
	    static createFrom(source: any = {}) {
	        return new UI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DefaultView = source["DefaultView"];
	        this.TimeZone = source["TimeZone"];
	        this.Keybindings = source["Keybindings"];
	        this.Board = this.convertValues(source["Board"], UIBoard);
	        this.JiraView = this.convertValues(source["JiraView"], UIJiraView);
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
	export class ProjectTerminal {
	    env: Record<string, string>;
	    startup: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectTerminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.env = source["env"];
	        this.startup = source["startup"];
	    }
	}
	export class TerminalNotify {
	    Enabled?: boolean;
	    Sound?: boolean;
	    MinSeconds: number;
	    WaitSeconds: number;
	    Ignore: string[];
	
	    static createFrom(source: any = {}) {
	        return new TerminalNotify(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.Sound = source["Sound"];
	        this.MinSeconds = source["MinSeconds"];
	        this.WaitSeconds = source["WaitSeconds"];
	        this.Ignore = source["Ignore"];
	    }
	}
	export class QuickCommand {
	    label: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new QuickCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.command = source["command"];
	    }
	}
	export class Terminal {
	    Shell: string;
	    Env: Record<string, string>;
	    Startup: string[];
	    Commands: QuickCommand[];
	    Watch: string[];
	    Notify: TerminalNotify;
	    Projects: Record<string, ProjectTerminal>;
	
	    static createFrom(source: any = {}) {
	        return new Terminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Shell = source["Shell"];
	        this.Env = source["Env"];
	        this.Startup = source["Startup"];
	        this.Commands = this.convertValues(source["Commands"], QuickCommand);
	        this.Watch = source["Watch"];
	        this.Notify = this.convertValues(source["Notify"], TerminalNotify);
	        this.Projects = this.convertValues(source["Projects"], ProjectTerminal, true);
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
	export class GitHub {
	    Enabled?: boolean;
	    Auth: string;
	    APIURL: string;
	    PollSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new GitHub(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.Auth = source["Auth"];
	        this.APIURL = source["APIURL"];
	        this.PollSeconds = source["PollSeconds"];
	    }
	}
	export class Review {
	    RequiredApprovals: number;
	
	    static createFrom(source: any = {}) {
	        return new Review(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RequiredApprovals = source["RequiredApprovals"];
	    }
	}
	export class PR {
	    TitleTemplate: string;
	    BodyTemplate: string;
	    MergeMethod: string;
	
	    static createFrom(source: any = {}) {
	        return new PR(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TitleTemplate = source["TitleTemplate"];
	        this.BodyTemplate = source["BodyTemplate"];
	        this.MergeMethod = source["MergeMethod"];
	    }
	}
	export class Dev {
	    ScanDirs: string[];
	    KeyPattern: string;
	    Branch: Branch;
	    PR: PR;
	    Checks: Checks;
	    Review: Review;
	    GitHub: GitHub;
	
	    static createFrom(source: any = {}) {
	        return new Dev(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ScanDirs = source["ScanDirs"];
	        this.KeyPattern = source["KeyPattern"];
	        this.Branch = this.convertValues(source["Branch"], Branch);
	        this.PR = this.convertValues(source["PR"], PR);
	        this.Checks = this.convertValues(source["Checks"], Checks);
	        this.Review = this.convertValues(source["Review"], Review);
	        this.GitHub = this.convertValues(source["GitHub"], GitHub);
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
	export class CreateDefault {
	    AssignToMe?: boolean;
	    Sprint: string;
	    FixVersion: string;
	    AffectsVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateDefault(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AssignToMe = source["AssignToMe"];
	        this.Sprint = source["Sprint"];
	        this.FixVersion = source["FixVersion"];
	        this.AffectsVersion = source["AffectsVersion"];
	    }
	}
	export class Defaults {
	    EpicProject: string;
	    Priority: string;
	    Create: CreateDefault;
	
	    static createFrom(source: any = {}) {
	        return new Defaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.EpicProject = source["EpicProject"];
	        this.Priority = source["Priority"];
	        this.Create = this.convertValues(source["Create"], CreateDefault);
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
	export class Team {
	    Members: string[];
	
	    static createFrom(source: any = {}) {
	        return new Team(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Members = source["Members"];
	    }
	}
	export class Links {
	    Blocking: string;
	    Visible: string[];
	
	    static createFrom(source: any = {}) {
	        return new Links(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Blocking = source["Blocking"];
	        this.Visible = source["Visible"];
	    }
	}
	export class Workflow {
	    States: Record<string, string>;
	    Order: string[];
	    DefaultResolution: string;
	
	    static createFrom(source: any = {}) {
	        return new Workflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.States = source["States"];
	        this.Order = source["Order"];
	        this.DefaultResolution = source["DefaultResolution"];
	    }
	}
	export class CustomField {
	    ID: string;
	    Name: string;
	    Show: string[];
	    Role: string;
	    Default: string;
	    DefaultFor: string[];
	    RequiredOn: string[];
	    Label: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Show = source["Show"];
	        this.Role = source["Role"];
	        this.Default = source["Default"];
	        this.DefaultFor = source["DefaultFor"];
	        this.RequiredOn = source["RequiredOn"];
	        this.Label = source["Label"];
	    }
	}
	export class Fields {
	    StoryPoints: string;
	    Parent: string;
	    InitiativeParent: string;
	    Sprint: string;
	    Priorities: string[];
	    Team: string;
	    FixVersion: string;
	    Custom: CustomField[];
	
	    static createFrom(source: any = {}) {
	        return new Fields(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.StoryPoints = source["StoryPoints"];
	        this.Parent = source["Parent"];
	        this.InitiativeParent = source["InitiativeParent"];
	        this.Sprint = source["Sprint"];
	        this.Priorities = source["Priorities"];
	        this.Team = source["Team"];
	        this.FixVersion = source["FixVersion"];
	        this.Custom = this.convertValues(source["Custom"], CustomField);
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
	export class IssueTypeRole {
	    ID: string;
	    Name: string;
	    Role: string;
	
	    static createFrom(source: any = {}) {
	        return new IssueTypeRole(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Role = source["Role"];
	    }
	}
	export class Hierarchy {
	    InitiativeProject: string;
	    Types: IssueTypeRole[];
	    Children: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new Hierarchy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.InitiativeProject = source["InitiativeProject"];
	        this.Types = this.convertValues(source["Types"], IssueTypeRole);
	        this.Children = source["Children"];
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
	export class Repo {
	    Name: string;
	    Host: string;
	    Path: string;
	    BaseBranch: string;
	    Components: string[];
	
	    static createFrom(source: any = {}) {
	        return new Repo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Host = source["Host"];
	        this.Path = source["Path"];
	        this.BaseBranch = source["BaseBranch"];
	        this.Components = source["Components"];
	    }
	}
	export class Project {
	    Key: string;
	    Board: number;
	    Repos: Repo[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Key = source["Key"];
	        this.Board = source["Board"];
	        this.Repos = this.convertValues(source["Repos"], Repo);
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
	export class MCP {
	    Command: string[];
	    URL: string;
	
	    static createFrom(source: any = {}) {
	        return new MCP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Command = source["Command"];
	        this.URL = source["URL"];
	    }
	}
	export class WriteBack {
	    RemoteLinks?: boolean;
	    TransitionComments: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WriteBack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RemoteLinks = source["RemoteLinks"];
	        this.TransitionComments = source["TransitionComments"];
	    }
	}
	export class Sync {
	    IntervalSeconds: number;
	    BurstSeconds: number;
	    FullResyncMinutes: number;
	    DiscoveryTTLHours: number;
	    Webhooks: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Sync(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.IntervalSeconds = source["IntervalSeconds"];
	        this.BurstSeconds = source["BurstSeconds"];
	        this.FullResyncMinutes = source["FullResyncMinutes"];
	        this.DiscoveryTTLHours = source["DiscoveryTTLHours"];
	        this.Webhooks = source["Webhooks"];
	    }
	}
	export class TLS {
	    CAFile: string;
	    AllowInsecure: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TLS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CAFile = source["CAFile"];
	        this.AllowInsecure = source["AllowInsecure"];
	    }
	}
	export class HTTP {
	    TimeoutSeconds: number;
	    MaxConcurrency: number;
	
	    static createFrom(source: any = {}) {
	        return new HTTP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TimeoutSeconds = source["TimeoutSeconds"];
	        this.MaxConcurrency = source["MaxConcurrency"];
	    }
	}
	export class Jira {
	    Site: string;
	    Deployment: string;
	    CloudID: string;
	    Transport: string;
	    Auth: Auth;
	    HTTP: HTTP;
	    Proxy: string;
	    TLS: TLS;
	    Sync: Sync;
	    WriteBack: WriteBack;
	    MCP: MCP;
	
	    static createFrom(source: any = {}) {
	        return new Jira(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Site = source["Site"];
	        this.Deployment = source["Deployment"];
	        this.CloudID = source["CloudID"];
	        this.Transport = source["Transport"];
	        this.Auth = this.convertValues(source["Auth"], Auth);
	        this.HTTP = this.convertValues(source["HTTP"], HTTP);
	        this.Proxy = source["Proxy"];
	        this.TLS = this.convertValues(source["TLS"], TLS);
	        this.Sync = this.convertValues(source["Sync"], Sync);
	        this.WriteBack = this.convertValues(source["WriteBack"], WriteBack);
	        this.MCP = this.convertValues(source["MCP"], MCP);
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
	export class Config {
	    Version: number;
	    TeamFile: string;
	    Jira: Jira;
	    Projects: Project[];
	    Hierarchy: Hierarchy;
	    Fields: Fields;
	    Workflow: Workflow;
	    Links: Links;
	    Team: Team;
	    Defaults: Defaults;
	    Dev: Dev;
	    Automation: Automation;
	    Terminal: Terminal;
	    Agent: Agent;
	    UI: UI;
	    Log: Log;
	    Sprints: Sprints;
	    Versions: Versions;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Version = source["Version"];
	        this.TeamFile = source["TeamFile"];
	        this.Jira = this.convertValues(source["Jira"], Jira);
	        this.Projects = this.convertValues(source["Projects"], Project);
	        this.Hierarchy = this.convertValues(source["Hierarchy"], Hierarchy);
	        this.Fields = this.convertValues(source["Fields"], Fields);
	        this.Workflow = this.convertValues(source["Workflow"], Workflow);
	        this.Links = this.convertValues(source["Links"], Links);
	        this.Team = this.convertValues(source["Team"], Team);
	        this.Defaults = this.convertValues(source["Defaults"], Defaults);
	        this.Dev = this.convertValues(source["Dev"], Dev);
	        this.Automation = this.convertValues(source["Automation"], Automation);
	        this.Terminal = this.convertValues(source["Terminal"], Terminal);
	        this.Agent = this.convertValues(source["Agent"], Agent);
	        this.UI = this.convertValues(source["UI"], UI);
	        this.Log = this.convertValues(source["Log"], Log);
	        this.Sprints = this.convertValues(source["Sprints"], Sprints);
	        this.Versions = this.convertValues(source["Versions"], Versions);
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
	
	
	
	
	
	
	
	
	
	
	
	export class Problem {
	    severity: string;
	    file?: string;
	    line?: number;
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.file = source["file"];
	        this.line = source["line"];
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class Layers {
	    PersonalPath: string;
	    TeamPath: string;
	    Personal?: Config;
	    Team?: Config;
	    Effective?: Config;
	    Problems: Problem[];
	
	    static createFrom(source: any = {}) {
	        return new Layers(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PersonalPath = source["PersonalPath"];
	        this.TeamPath = source["TeamPath"];
	        this.Personal = this.convertValues(source["Personal"], Config);
	        this.Team = this.convertValues(source["Team"], Config);
	        this.Effective = this.convertValues(source["Effective"], Config);
	        this.Problems = this.convertValues(source["Problems"], Problem);
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

export namespace devsync {
	
	export class Status {
	    state: string;
	    host: string;
	    user: string;
	    repos: string[];
	    error: string;
	    lastSync: string;
	    prs: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.host = source["host"];
	        this.user = source["user"];
	        this.repos = source["repos"];
	        this.error = source["error"];
	        this.lastSync = source["lastSync"];
	        this.prs = source["prs"];
	    }
	}

}

export namespace flow {
	
	export class Check {
	    name: string;
	    state: string;
	    url?: string;
	    required?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	        this.url = source["url"];
	        this.required = source["required"];
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
	    repo?: string;
	    draft?: boolean;
	    author?: string;
	    head?: string;
	    base?: string;
	    requiredApprovals?: number;
	    approvedBy?: string[];
	    changesRequested?: string[];
	    mergedAt?: string;
	    updatedAt?: string;
	    live?: boolean;
	
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
	        this.repo = source["repo"];
	        this.draft = source["draft"];
	        this.author = source["author"];
	        this.head = source["head"];
	        this.base = source["base"];
	        this.requiredApprovals = source["requiredApprovals"];
	        this.approvedBy = source["approvedBy"];
	        this.changesRequested = source["changesRequested"];
	        this.mergedAt = source["mergedAt"];
	        this.updatedAt = source["updatedAt"];
	        this.live = source["live"];
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
	    live: boolean;
	
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
	        this.live = source["live"];
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
	export class Naming {
	    branchTemplate: string;
	    prefix: Record<string, string>;
	    slugWords: number;
	    prTitle: string;
	    prBody: string;
	    mergeMethod: string;
	
	    static createFrom(source: any = {}) {
	        return new Naming(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.branchTemplate = source["branchTemplate"];
	        this.prefix = source["prefix"];
	        this.slugWords = source["slugWords"];
	        this.prTitle = source["prTitle"];
	        this.prBody = source["prBody"];
	        this.mergeMethod = source["mergeMethod"];
	    }
	}
	export class Policy {
	    anyAssignee: boolean;
	    anyAuthor: boolean;
	    dryRun: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.anyAssignee = source["anyAssignee"];
	        this.anyAuthor = source["anyAuthor"];
	        this.dryRun = source["dryRun"];
	    }
	}
	
	export class Rule {
	    id: string;
	    trigger: string;
	    label: string;
	    toStatus: string;
	    enabled: boolean;
	    projects: string[];
	    issueTypes: string[];
	
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
	        this.projects = source["projects"];
	        this.issueTypes = source["issueTypes"];
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
	export class SettingsProblem {
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsProblem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class TerminalSettings {
	    shell: string;
	    env: Record<string, string>;
	    startup: string[];
	    commands: config.QuickCommand[];
	    projects: Record<string, config.ProjectTerminal>;
	
	    static createFrom(source: any = {}) {
	        return new TerminalSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.shell = source["shell"];
	        this.env = source["env"];
	        this.startup = source["startup"];
	        this.commands = this.convertValues(source["commands"], config.QuickCommand);
	        this.projects = this.convertValues(source["projects"], config.ProjectTerminal, true);
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
	export class Settings {
	    naming: flow.Naming;
	    terminal: TerminalSettings;
	    path: string;
	    teamPath: string;
	    writable: boolean;
	    saved: boolean;
	    problems: SettingsProblem[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.naming = this.convertValues(source["naming"], flow.Naming);
	        this.terminal = this.convertValues(source["terminal"], TerminalSettings);
	        this.path = source["path"];
	        this.teamPath = source["teamPath"];
	        this.writable = source["writable"];
	        this.saved = source["saved"];
	        this.problems = this.convertValues(source["problems"], SettingsProblem);
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
	    naming: flow.Naming;
	    commands: config.QuickCommand[];
	    policy: flow.Policy;
	
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
	        this.naming = this.convertValues(source["naming"], flow.Naming);
	        this.commands = this.convertValues(source["commands"], config.QuickCommand);
	        this.policy = this.convertValues(source["policy"], flow.Policy);
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
	export class TermHistory {
	    data: string;
	    end: number;
	
	    static createFrom(source: any = {}) {
	        return new TermHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = source["data"];
	        this.end = source["end"];
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
	export class BoardColumn {
	    name: string;
	    keys: string[];
	    statuses: string[];
	
	    static createFrom(source: any = {}) {
	        return new BoardColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.keys = source["keys"];
	        this.statuses = source["statuses"];
	    }
	}
	export class BoardInfo {
	    id: number;
	    name: string;
	    type: string;
	    project?: string;
	
	    static createFrom(source: any = {}) {
	        return new BoardInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.project = source["project"];
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
	export class BoardView {
	    board: BoardInfo;
	    sprint?: SprintInfo;
	    columns: BoardColumn[];
	    issues: Issue[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BoardView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.board = this.convertValues(source["board"], BoardInfo);
	        this.sprint = this.convertValues(source["sprint"], SprintInfo);
	        this.columns = this.convertValues(source["columns"], BoardColumn);
	        this.issues = this.convertValues(source["issues"], Issue);
	        this.truncated = source["truncated"];
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

