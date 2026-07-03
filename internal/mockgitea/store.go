// Package mockgitea is an in-memory fake Gitea server used by --mock and by
// end-to-end tests. Handlers speak the subset of the Gitea REST API that
// internal/gitea's client consumes; contract tests drive the real client
// against this server.
package mockgitea

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// User mirrors the subset of Gitea's user object the client reads.
type User struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// Label mirrors a repository label definition.
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Milestone mirrors a repository milestone definition.
type Milestone struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Repo mirrors the subset of Gitea's repository object the client reads,
// including the merge-strategy flags internal/gitea.mergeCapabilitiesFromRepository
// consumes off GetRepo (see internal/gitea/mutation.go and its
// TestMergeCapabilitiesMapsRepositorySettings).
type Repo struct {
	ID                        int64  `json:"id"`
	Name                      string `json:"name"`
	Owner                     *User  `json:"owner"`
	FullName                  string `json:"full_name"`
	HTMLURL                   string `json:"html_url"`
	AllowMergeCommits         bool   `json:"allow_merge_commits"`
	AllowRebase               bool   `json:"allow_rebase"`
	AllowRebaseExplicit       bool   `json:"allow_rebase_explicit"`
	AllowSquashMerge          bool   `json:"allow_squash_merge"`
	AllowFastForwardOnlyMerge bool   `json:"allow_fast_forward_only_merge"`
	DefaultMergeStyle         string `json:"default_merge_style"`
}

// CommitStatus mirrors one entry of a combined commit status.
type CommitStatus struct {
	Status  string `json:"status"`
	Context string `json:"context"`
	URL     string `json:"target_url"`
}

// Review mirrors a submitted pull request review.
type Review struct {
	ID       int64     `json:"id"`
	State    string    `json:"state"`
	Body     string    `json:"body"`
	Reviewer *User     `json:"user"`
	Created  time.Time `json:"submitted_at"`
}

// Comment mirrors an issue/PR comment.
type Comment struct {
	ID      int64     `json:"id"`
	Body    string    `json:"body"`
	Author  *User     `json:"user"`
	Created time.Time `json:"created_at"`
	Updated time.Time `json:"updated_at"`
}

// Pull is a denormalized pull request record. Fields tagged "-" are
// store-internal bookkeeping, not part of the Gitea wire shape.
type Pull struct {
	ID           int64           `json:"id"`
	Number       int64           `json:"number"`
	RepoFullName string          `json:"-"`
	Title        string          `json:"title"`
	Body         string          `json:"body"`
	State        string          `json:"state"`
	Merged       bool            `json:"merged"`
	Mergeable    bool            `json:"mergeable"`
	Draft        bool            `json:"draft"`
	Author       *User           `json:"user"`
	Labels       []*Label        `json:"labels"`
	Milestone    *Milestone      `json:"milestone,omitempty"`
	Assignees    []*User         `json:"assignees"`
	Reviewers    []*User         `json:"requested_reviewers"`
	HeadRef      string          `json:"-"`
	HeadSHA      string          `json:"-"`
	BaseRef      string          `json:"-"`
	CommentCount int64           `json:"comments"`
	Created      time.Time       `json:"created_at"`
	Updated      time.Time       `json:"updated_at"`
	HTMLURL      string          `json:"html_url"`
	Diff         string          `json:"-"`
	Statuses     []*CommitStatus `json:"-"`
	Reviews      []*Review       `json:"-"`
}

// Issue is a denormalized issue record. Fields tagged "-" are store-internal
// bookkeeping, not part of the Gitea wire shape.
type Issue struct {
	ID           int64           `json:"id"`
	Number       int64           `json:"number"`
	RepoFullName string          `json:"-"`
	Title        string          `json:"title"`
	Body         string          `json:"body"`
	State        string          `json:"state"`
	Author       *User           `json:"user"`
	Labels       []*Label        `json:"labels"`
	Milestone    *Milestone      `json:"milestone,omitempty"`
	Assignees    []*User         `json:"assignees"`
	CommentCount int64           `json:"comments"`
	Created      time.Time       `json:"created_at"`
	Updated      time.Time       `json:"updated_at"`
	HTMLURL      string          `json:"html_url"`
	Subscribers  map[string]bool `json:"-"`
}

// Notification is a denormalized notification thread record. Fields tagged
// "-" are store-internal bookkeeping filled in by handlers, not decoded
// directly off a single Gitea JSON shape (the real API nests them under
// subject/repository).
type Notification struct {
	ID       int64     `json:"id"`
	Unread   bool      `json:"unread"`
	Pinned   bool      `json:"pinned"`
	Title    string    `json:"-"`
	Type     string    `json:"-"`
	State    string    `json:"-"`
	RepoFull string    `json:"-"`
	URL      string    `json:"-"`
	Updated  time.Time `json:"updated_at"`
}

// ActionRun mirrors the fields internal/gitea/actions.go's tolerant decode
// (rawActionRun) actually reads off a workflow run: id/display_title/
// workflow_name/event/status/head_branch/head_sha/html_url/created_at/
// updated_at, plus a nested actor.login. RepoFullName and Jobs are
// store-internal bookkeeping, not serialized.
type ActionRun struct {
	ID           int64        `json:"id"`
	DisplayTitle string       `json:"display_title"`
	WorkflowName string       `json:"workflow_name"`
	Event        string       `json:"event"`
	Status       string       `json:"status"`
	HeadBranch   string       `json:"head_branch"`
	HeadSHA      string       `json:"head_sha"`
	Actor        *User        `json:"actor,omitempty"`
	Created      time.Time    `json:"created_at"`
	Updated      time.Time    `json:"updated_at"`
	HTMLURL      string       `json:"html_url"`
	RepoFullName string       `json:"-"`
	Jobs         []*ActionJob `json:"-"`
}

// ActionJob mirrors the fields internal/gitea/actions.go's tolerant decode
// (rawActionJob) actually reads off a workflow job: id/name/status/
// started_at/completed_at. Logs are fetched via a separate plain-text
// endpoint, not this JSON shape, hence "-".
type ActionJob struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Status  string    `json:"status"`
	Started time.Time `json:"started_at"`
	Stopped time.Time `json:"completed_at"`
	Logs    string    `json:"-"`
}

// Store is an in-memory, mutex-guarded fake of a Gitea server's data. It has
// no HTTP surface of its own; handlers built on top of it (later tasks) speak
// the Gitea REST API and translate to/from these methods.
//
// Getter methods (Pull, Issue, Runs, ...) return live pointers into the
// store's interior, not copies. Mutator methods (MergePull, SetPullState,
// AddComment, ...) mutate that same interior in place and take the lock
// themselves for the duration of the mutation only. A caller that reads or
// serializes a getter's result (notably an HTTP handler marshaling a
// response) must do so inside WithLock, or a concurrent mutation can race
// with that read.
type Store struct {
	mu            sync.Mutex
	me            *User
	users         []*User
	repos         map[string]*Repo
	pulls         map[string][]*Pull
	issues        map[string][]*Issue
	comments      map[string][]*Comment
	notifications []*Notification
	runs          map[string][]*ActionRun
	labels        map[string][]*Label
	milestones    map[string][]*Milestone
	nextID        int64
}

// NewStore returns an empty Store seeded with a default authenticated user.
func NewStore() *Store {
	return &Store{
		me:         &User{ID: 1, Login: "gabor", FullName: "Gabor Barany"},
		repos:      make(map[string]*Repo),
		pulls:      make(map[string][]*Pull),
		issues:     make(map[string][]*Issue),
		comments:   make(map[string][]*Comment),
		runs:       make(map[string][]*ActionRun),
		labels:     make(map[string][]*Label),
		milestones: make(map[string][]*Milestone),
		nextID:     1000,
	}
}

// WithLock runs fn while holding the store lock. Getter results are live
// references into the store; callers that read or marshal them (notably the
// HTTP handlers) must do so inside WithLock. Mutator methods take the lock
// themselves and must NOT be called from inside fn (deadlock) — use the
// unexported "*Locked" lookups (e.g. pullLocked) instead, which assume the
// lock is already held.
func (s *Store) WithLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// id returns the next synthetic ID, monotonically increasing across all
// entity kinds. Callers must hold s.mu.
func (s *Store) id() int64 {
	s.nextID++
	return s.nextID
}

// key returns the comment-map key for one repo-scoped issue/PR number.
func key(repo string, num int64) string {
	return fmt.Sprintf("%s#%d", repo, num)
}

// Me returns the authenticated user.
func (s *Store) Me() *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.me
}

// SetMe replaces the authenticated user.
func (s *Store) SetMe(u *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.me = u
}

// AddUser registers a user the store knows about (for comment/review/assignee
// lookups by login).
func (s *Store) AddUser(u *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = append(s.users, u)
}

// Users returns every registered user.
func (s *Store) Users() []*User {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.users
}

// AddRepo registers a repository, keyed by its full name.
func (s *Store) AddRepo(r *Repo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == 0 {
		r.ID = s.id()
	}
	s.repos[r.FullName] = r
}

// RepoByFullName looks up a repository by "owner/name".
func (s *Store) RepoByFullName(fullName string) *Repo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repos[fullName]
}

// AddPull registers a pull request under its repo.
func (s *Store) AddPull(p *Pull) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == 0 {
		p.ID = s.id()
	}
	s.pulls[p.RepoFullName] = append(s.pulls[p.RepoFullName], p)
}

// Pull returns one pull request by repo and number, or nil.
func (s *Store) Pull(repo string, num int64) *Pull {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pullLocked(repo, num)
}

// pullLocked is Pull without taking the lock, for use from inside WithLock.
// Callers must hold s.mu.
func (s *Store) pullLocked(repo string, num int64) *Pull {
	for _, p := range s.pulls[repo] {
		if p.Number == num {
			return p
		}
	}
	return nil
}

// Pulls returns every pull request in one repo.
func (s *Store) Pulls(repo string) []*Pull {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pulls[repo]
}

// AllPulls returns every pull request across every repo, sorted by ID
// ascending. The sort makes the result deterministic across identically
// seeded stores: iteration over the repo-keyed map is otherwise randomized by
// Go, which would make two seed runs compare unequal element-by-element.
func (s *Store) AllPulls() []*Pull {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Pull
	for _, ps := range s.pulls {
		out = append(out, ps...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddIssue registers an issue under its repo. Subscribers is initialized to
// an empty (non-nil) map when the caller didn't set one, so the subscription
// handler can always write into it without a nil-map panic.
func (s *Store) AddIssue(i *Issue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i.ID == 0 {
		i.ID = s.id()
	}
	if i.Subscribers == nil {
		i.Subscribers = make(map[string]bool)
	}
	s.issues[i.RepoFullName] = append(s.issues[i.RepoFullName], i)
}

// Issue returns one issue by repo and number, or nil.
func (s *Store) Issue(repo string, num int64) *Issue {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range s.issues[repo] {
		if i.Number == num {
			return i
		}
	}
	return nil
}

// Issues returns every issue in one repo.
func (s *Store) Issues(repo string) []*Issue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issues[repo]
}

// AllIssues returns every issue across every repo, sorted by ID ascending.
// The sort makes the result deterministic across identically seeded stores:
// iteration over the repo-keyed map is otherwise randomized by Go, which
// would make two seed runs compare unequal element-by-element.
func (s *Store) AllIssues() []*Issue {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Issue
	for _, is := range s.issues {
		out = append(out, is...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddComment appends a comment to an issue or pull request's thread and bumps
// the matching pull/issue's comment count and updated time. The author is
// resolved by login among registered users and the authenticated user,
// falling back to a bare User with just the login set.
//
// Gitea numbers issues and pull requests out of one shared per-repo index
// space, so a given (repo, num) matches at most one row across s.pulls and
// s.issues — never both. Seed/demo data must respect that invariant, or this
// method will (harmlessly, but wrongly) bump both.
func (s *Store) AddComment(repo string, num int64, login, body string) *Comment {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	c := &Comment{
		ID:      s.id(),
		Body:    body,
		Author:  s.resolveUserLocked(login),
		Created: now,
		Updated: now,
	}
	k := key(repo, num)
	s.comments[k] = append(s.comments[k], c)

	for _, p := range s.pulls[repo] {
		if p.Number == num {
			p.CommentCount++
			p.Updated = now
		}
	}
	for _, i := range s.issues[repo] {
		if i.Number == num {
			i.CommentCount++
			i.Updated = now
		}
	}
	return c
}

// resolveUserLocked finds a registered user (or the authenticated user) by
// login, falling back to a bare User with just the login set. Callers must
// hold s.mu.
func (s *Store) resolveUserLocked(login string) *User {
	if s.me != nil && s.me.Login == login {
		return s.me
	}
	for _, u := range s.users {
		if u.Login == login {
			return u
		}
	}
	return &User{Login: login}
}

// Comments returns every comment on one repo-scoped issue/PR number.
func (s *Store) Comments(repo string, num int64) []*Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.comments[key(repo, num)]
}

// AddNotification registers a notification thread.
func (s *Store) AddNotification(n *Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n.ID == 0 {
		n.ID = s.id()
	}
	s.notifications = append(s.notifications, n)
}

// Notifications returns every notification thread.
func (s *Store) Notifications() []*Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notifications
}

// NotificationByID returns one notification thread by ID, or nil.
func (s *Store) NotificationByID(id int64) *Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notificationByIDLocked(id)
}

func (s *Store) notificationByIDLocked(id int64) *Notification {
	for _, n := range s.notifications {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// SetNotificationStatus applies one of "read", "unread", "pinned", or
// "unpinned" to a notification thread. It errors if the thread is unknown or
// the status is not one of those four.
func (s *Store) SetNotificationStatus(id int64, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.notificationByIDLocked(id)
	if n == nil {
		return fmt.Errorf("mockgitea: unknown notification %d", id)
	}
	switch status {
	case "read":
		n.Unread = false
	case "unread":
		n.Unread = true
	case "pinned":
		n.Pinned = true
	case "unpinned":
		n.Pinned = false
	default:
		return fmt.Errorf("mockgitea: unknown notification status %q", status)
	}
	return nil
}

// MarkAllNotificationsRead marks every unread notification thread as read.
func (s *Store) MarkAllNotificationsRead() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.notifications {
		n.Unread = false
	}
}

// MergePull marks a pull request merged and closed. It errors if the pull is
// unknown. style and deleteBranch are accepted (and currently unused) for
// later tasks — e.g. an HTTP handler that echoes the merge style back, or
// simulated branch deletion.
func (s *Store) MergePull(repo string, num int64, style string, deleteBranch bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pulls[repo] {
		if p.Number == num {
			p.Merged = true
			p.State = "closed"
			p.Updated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("mockgitea: unknown pull %s#%d", repo, num)
}

// SetPullState opens or closes a pull request. It errors if the pull is
// unknown.
func (s *Store) SetPullState(repo string, num int64, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pulls[repo] {
		if p.Number == num {
			p.State = state
			p.Updated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("mockgitea: unknown pull %s#%d", repo, num)
}

// SetIssueState opens or closes an issue. It errors if the issue is unknown.
func (s *Store) SetIssueState(repo string, num int64, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range s.issues[repo] {
		if i.Number == num {
			i.State = state
			i.Updated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("mockgitea: unknown issue %s#%d", repo, num)
}

// SetPullDraft flips a pull request's draft flag. It errors if the pull is
// unknown.
func (s *Store) SetPullDraft(repo string, num int64, draft bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pulls[repo] {
		if p.Number == num {
			p.Draft = draft
			p.Updated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("mockgitea: unknown pull %s#%d", repo, num)
}

// AddRun registers an Actions workflow run under its repo.
func (s *Store) AddRun(run *ActionRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == 0 {
		run.ID = s.id()
	}
	s.runs[run.RepoFullName] = append(s.runs[run.RepoFullName], run)
}

// Runs returns every Actions workflow run in one repo.
func (s *Store) Runs(repo string) []*ActionRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[repo]
}

// RunByID returns one Actions workflow run by repo and ID, or nil.
func (s *Store) RunByID(repo string, id int64) *ActionRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs[repo] {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// SetRunStatus updates one Actions workflow run's status. It errors if the
// run is unknown.
func (s *Store) SetRunStatus(repo string, id int64, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs[repo] {
		if r.ID == id {
			r.Status = status
			r.Updated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("mockgitea: unknown action run %s#%d", repo, id)
}

// AddLabelDef registers a label definition available in one repo.
func (s *Store) AddLabelDef(repo string, l *Label) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.ID == 0 {
		l.ID = s.id()
	}
	s.labels[repo] = append(s.labels[repo], l)
}

// LabelDefs returns every label definition available in one repo.
func (s *Store) LabelDefs(repo string) []*Label {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.labels[repo]
}

// AddMilestoneDef registers a milestone definition available in one repo.
func (s *Store) AddMilestoneDef(repo string, m *Milestone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ID == 0 {
		m.ID = s.id()
	}
	s.milestones[repo] = append(s.milestones[repo], m)
}

// MilestoneDefs returns every milestone definition available in one repo.
func (s *Store) MilestoneDefs(repo string) []*Milestone {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.milestones[repo]
}
