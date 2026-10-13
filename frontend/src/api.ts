// Thin typed wrapper over the generated Wails bindings.
import * as App from '../wailsjs/go/main/App'
import type { BoardInfo, JiraBoard, SprintInfo, Connection, GitHubStatus, FieldPatch, FieldSpec, Flow, Issue, IssueType, NewIssue, Rule, Snapshot, StepResult, Transition, FlowEvent } from './types'

// Every backend call goes through call(): if a binding is missing (backend older
// than the UI, bridge not ready) it throws synchronously, which would escape
// .catch() and crash the render. call() turns that into a rejected promise.
const call = <T,>(fn: () => Promise<unknown>) => Promise.resolve().then(fn) as Promise<T>

export const api = {
  snapshot: () => call<Snapshot>(() => App.Snapshot()),
  issue: (key: string) => call<Issue>(() => App.GetIssue(key)),
  transitions: (key: string) => call<Transition[]>(() => App.GetTransitions(key)),
  transition: (key: string, to: string) => call<Issue>(() => App.TransitionIssue(key, to)),
  comment: (key: string, body: string) => call<Issue>(() => App.AddComment(key, body)),
  createIssue: (input: NewIssue) => call<Issue>(() => App.CreateIssue(input as never)),
  updateFields: (key: string, p: FieldPatch) => call<Issue>(() => App.UpdateFields(key, p as never)),
  fieldSpecs: (key: string) => call<FieldSpec[]>(() => App.GetFieldSpecs(key)),
  createFieldSpecs: (project: string, type: IssueType) => call<FieldSpec[]>(() => App.GetCreateFieldSpecs(project, type)),
  link: (from: string, type: string, to: string) => call<Issue[]>(() => App.LinkIssues(from, type, to)),
  unlink: (from: string, type: string, to: string) => call<Issue[]>(() => App.UnlinkIssues(from, type, to)),
  flow: (key: string) => call<Flow>(() => App.GetFlow(key)),
  startWork: (key: string) => call<StepResult>(() => App.StartWork(key)),
  nextEvent: (key: string) => call<StepResult>(() => App.SimulateNextEvent(key)),
  resetFlow: (key: string) => call<StepResult>(() => App.ResetFlow(key)),
  activity: () => call<FlowEvent[]>(() => App.GetActivity()),
  toggleRule: (id: string) => call<Rule[]>(() => App.ToggleRule(id)),
  setRuleStatus: (id: string, status: string) => call<Rule[]>(() => App.SetRuleStatus(id, status)),
  openURL: (url: string) => call<void>(() => App.OpenURL(url)),
  copy: (text: string) => call<boolean>(() => App.CopyToClipboard(text)),
  logInfo: () => call<{ dir: string; file: string; previousCrash: string }>(() => App.GetLogInfo()),
  openLogFolder: () => call<void>(() => App.OpenLogFolder()),
  connection: () => call<Connection>(() => App.GetConnection()),
  searchBoards: (q: string) => call<BoardInfo[]>(() => App.SearchBoards(q)),
  boardSprints: (boardId: number) => call<SprintInfo[]>(() => App.GetBoardSprints(boardId)),
  board: (boardId: number, sprintId: number) => call<JiraBoard>(() => App.GetBoard(boardId, sprintId)),
  githubStatus: () => call<GitHubStatus>(() => App.GetGitHubStatus()),
  termOpen: (key: string, cols: number, rows: number) => call<boolean>(() => App.TermOpen(key, cols, rows)),
  termWrite: (key: string, data: string) => call<void>(() => App.TermWrite(key, data)),
  termResize: (key: string, cols: number, rows: number) => call<void>(() => App.TermResize(key, cols, rows)),
  termClose: (key: string) => call<void>(() => App.TermClose(key)),
}
