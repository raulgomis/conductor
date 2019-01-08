package types

import (
	"fmt"
	"strings"

	"github.com/Nextdoor/conductor/shared/settings"
)

// Special settings table which should only have one row.
type Config struct {
	ID uint64 `orm:"pk;auto;column(id)" json:"-"`

	// Current mode
	Mode Mode `json:"mode"`

	// JSON string for configuration, like the CloseTime config.
	// See: shared/types/options.go.
	Options Options `json:"options"`
}

var DefaultConfig *Config = &Config{ID: 1, Mode: Schedule, Options: DefaultOptions}

type Train struct {
	ID               uint64        `orm:"pk;auto;column(id)" json:"id,string"`
	Engineer         *User         `orm:"rel(fk);null" json:"engineer"`
	CreatedAt        Time          `orm:"auto_now_add" json:"created_at"`
	DeployedAt       Time          `orm:"null" json:"deployed_at"`
	CancelledAt      Time          `orm:"null" json:"cancelled_at"`
	Closed           bool          `json:"closed"`
	ScheduleOverride bool          `json:"schedule_override"`
	Blocked          bool          `json:"blocked"`
	BlockedReason    *string       `orm:"null" json:"blocked_reason"`
	Branch           string        `json:"branch"`
	HeadSHA          string        `orm:"column(head_sha)" json:"head_sha"`
	TailSHA          string        `orm:"column(tail_sha)" json:"tail_sha"`
	Commits          []*Commit     `orm:"rel(m2m)" json:"commits"`      // Commits on this train.
	Tickets          []*Ticket     `orm:"reverse(many)" json:"tickets"` // Who's got a ticket to ride?
	ActivePhases     *PhaseGroup   `orm:"rel(fk)" json:"active_phases"`
	AllPhaseGroups   []*PhaseGroup `orm:"reverse(many)" json:"all_phase_groups"`

	// Computed fields
	ActivePhase         PhaseType `orm:"-" json:"active_phase"`
	LastDeliveredSHA    *string   `orm:"-" json:"last_delivered_sha"` // SHA for last successful delivery.
	PreviousID          *uint64   `orm:"-" json:"previous_id,string"`
	NextID              *uint64   `orm:"-" json:"next_id,string"`
	NotDeployableReason *string   `orm:"-" json:"not_deployable_reason"`
	Done                bool      `orm:"-" json:"done"`
	PreviousTrainDone   bool      `orm:"-" json:"previous_train_done"`
	CanRollback         bool      `orm:"-" json:"can_rollback"`
}

type Phase struct {
	ID          uint64    `orm:"pk;auto;column(id)" json:"id,string"`
	StartedAt   Time      `orm:"null" json:"started_at"`
	CompletedAt Time      `orm:"null" json:"completed_at"`
	Type        PhaseType `json:"type"` // delivery|verification|deploy
	Error       string    `orm:"null" json:"error"`
	Jobs        Jobs      `orm:"reverse(many)" json:"jobs"`

	// Computed fields
	PhaseGroup *PhaseGroup `orm:"-" json:"-"`
	Train      *Train      `orm:"-" json:"-"`
}

type PhaseGroup struct {
	ID           uint64 `orm:"pk;auto;column(id)" json:"id,string"`
	HeadSHA      string `orm:"column(head_sha)" json:"head_sha"`
	Delivery     *Phase `orm:"rel(fk)" json:"delivery"`
	Verification *Phase `orm:"rel(fk)" json:"verification"`
	Deploy       *Phase `orm:"rel(fk)" json:"deploy"`

	Train *Train `orm:"rel(fk);null" json:"-"`
}

type Job struct {
	ID          uint64    `orm:"pk;auto;column(id)" json:"id,string"`
	StartedAt   Time      `orm:"null" json:"started_at"`
	CompletedAt Time      `orm:"null" json:"completed_at"`
	URL         *string   `orm:"column(url);null" json:"url"` // Link to this job
	Name        string    `json:"name"`                       // e.g. Delivery, Test, Build
	Result      JobResult `json:"result"`                     // Exit status
	Metadata    string    `orm:"null" json:"metadata"`        // JSON data
	Phase       *Phase    `orm:"rel(fk)" json:"-"`
}

type Commit struct {
	ID          uint64 `orm:"pk;auto;column(id)" json:"id,string"`
	CreatedAt   Time   `orm:"auto_now_add;null" json:"created_at"`
	SHA         string `orm:"unique;column(sha)" json:"sha"`
	Message     string `json:"message"`
	Branch      string `json:"branch" orm:"-"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	URL         string `orm:"column(url)" json:"url"`
}

type Ticket struct {
	ID            uint64    `orm:"pk;auto;column(id)" json:"id,string"`
	Key           string    `json:"key"`
	Summary       string    `json:"summary"`
	AssigneeEmail string    `json:"assignee_email"`
	AssigneeName  string    `json:"assignee_name"`
	URL           string    `orm:"column(url)" json:"url"`
	CreatedAt     Time      `orm:"auto_now_add" json:"created_at"`
	ClosedAt      Time      `orm:"null" json:"closed_at"`
	DeletedAt     Time      `orm:"null" json:"deleted_at"`
	Commits       []*Commit `orm:"rel(m2m)" json:"commits"`
	Train         *Train    `orm:"rel(fk)" json:"-"`
}

type User struct {
	ID        uint64 `orm:"pk;auto;column(id)" json:"id,string"`
	CreatedAt Time   `orm:"auto_now_add" json:"created_at"`
	Name      string `json:"name"`
	Email     string `orm:"unique" json:"email"`
	IsViewer  bool   `orm:"-" json:"is_viewer"`
	IsUser    bool   `orm:"-" json:"is_user"`
	IsAdmin   bool   `orm:"-" json:"is_admin"`
}

type Search struct {
	Params  map[string]string `json:"params"`
	Results interface{}       `json:"results"`
}

func (_ *Ticket) TableUnique() [][]string {
	return [][]string{
		// Unique constraint on key + train id.
		// Ticket might be shared between trains, but the same ticket should never be on the same train twice.
		[]string{"Key", "Train"},
	}
}

type Metadata struct {
	Namespace string `orm:"pk;unique" json:"key"`
	Data      string `orm:"type(jsonb)" json:"data"`
}

func (commit *Commit) ShortSHA() string {
	return ShortSHA(commit.SHA)
}

func ShortSHA(sha string) string {
	min := 16
	if len(sha) < min {
		return sha
	}
	return sha[:min]
}

func (train *Train) Phase(phaseType PhaseType) *Phase {
	switch phaseType {
	case Delivery:
		return train.ActivePhases.Delivery
	case Verification:
		return train.ActivePhases.Verification
	case Deploy:
		return train.ActivePhases.Deploy
	}
	return nil
}

func (train *Train) SetActivePhase() {
	phases := train.ActivePhases
	if phases.Deploy.StartedAt.HasValue() {
		train.ActivePhase = Deploy
	} else if phases.Verification.StartedAt.HasValue() {
		train.ActivePhase = Verification
	} else {
		train.ActivePhase = Delivery
	}
}

func (train *Train) IsDeployable() bool {
	return train.NextID == nil &&
		train.PreviousTrainDone &&
		train.ActivePhase == Verification &&
		train.ActivePhases.Verification.IsComplete() &&
		train.Closed &&
		!train.Blocked &&
		!train.Done
}

func (train *Train) GetNotDeployableReason() *string {
	if train.IsDeployable() || train.ActivePhase != Verification || train.Done {
		return nil
	}

	var reason string
	if train.NextID != nil {
		reason = "Not the latest train."
	} else if train.ActivePhase == Verification && !train.ActivePhases.Verification.IsComplete() {
		reason = "Waiting for verification."
	} else if !train.Closed {
		reason = "Train is not closed."
	} else if train.Blocked {
		if train.BlockedReason != nil {
			reason = fmt.Sprintf(
				"Train is blocked due to %s.",
				*train.BlockedReason)
		} else {
			reason = "Train is blocked."
		}
	} else if !train.PreviousTrainDone {
		reason = "Previous train is still deploying."
	}

	if reason == "" {
		return nil
	}
	return &reason
}

func DoesCommitNeedTicket(commit *Commit, commitsOnTickets map[string]struct{}) bool {
	_, found := commitsOnTickets[commit.SHA]
	// Exclude commits with tickets and commits marked for no verification.
	if found || commit.IsNoVerify() {
		return false
	}
	// Exclude users in the no staging pilot program, unless they manually
	// requested staging.
	if commit.IsNoStagingVerification() && !commit.IsNeedsStaging() {
		return false
	}
	return true
}

// Should this commit trigger slack notifications to its author regarding staging.
func (commit *Commit) DoesCommitNeedStagingNotification() bool {
	return !commit.IsNoStagingVerification() || commit.IsNeedsStaging()
}

func (commit *Commit) IsNoVerify() bool {
	return strings.Contains(commit.Message, "[no-verify]")
}

func (commit *Commit) IsNeedsStaging() bool {
	return strings.Contains(commit.Message, "[needs-staging]")
}

func (commit *Commit) IsNoStagingVerification() bool {
	if settings.NoStagingVerification {
		return true
	} else {
		return settings.IsNoStagingVerificationUser(commit.AuthorEmail)
	}
}

// Return includes head.
func (train *Train) CommitsSince(headSHA string) []*Commit {
	endIndex := len(train.Commits)
	for i := range train.Commits {
		if train.Commits[i].SHA == headSHA {
			endIndex = i + 1
			break
		}
	}

	return train.Commits[:endIndex]
}

// Return includes head but not tail.
func (train *Train) CommitsBetween(headSHA string, tailSHA string) []*Commit {
	commits := []*Commit{}
	inBetween := false
	for i := range train.Commits {
		commit := train.Commits[i]
		if inBetween {
			commits = append(commits, commit)
		}
		if commit.SHA == tailSHA {
			// After this commit, start adding all commits until we reach the current head sha.
			inBetween = true
		} else if commit.SHA == headSHA {
			// Done
			break
		}
	}
	return commits
}

func (train *Train) NewCommitsNeedingTickets(headSHA string) []*Commit {
	newCommits := make([]*Commit, 0)

	commitsOnTickets := make(map[string]struct{})
	for _, ticket := range train.Tickets {
		for _, commit := range ticket.Commits {
			commitsOnTickets[commit.SHA] = struct{}{}
		}
	}

	for _, commit := range train.CommitsSince(headSHA) {
		if DoesCommitNeedTicket(commit, commitsOnTickets) {
			newCommits = append(newCommits, commit)
		}
	}

	return newCommits
}

func (train *Train) IsDeploying() bool {
	return train.ActivePhases.Deploy.StartedAt.HasValue() && !train.ActivePhases.Deploy.CompletedAt.HasValue()
}

func (train *Train) IsDeployed() bool {
	return train.DeployedAt.HasValue()
}

func (train *Train) IsCancelled() bool {
	return train.CancelledAt.HasValue()
}

func (train *Train) IsDone() bool {
	return train.IsDeployed() || train.IsCancelled()
}

func (train *Train) GitReference() string {
	return fmt.Sprintf("%s-%s", train.Branch, ShortSHA(train.HeadSHA))
}

func (phase *Phase) IsComplete() bool {
	return phase.CompletedAt.HasValue()
}

func (phase *Phase) Before(phaseType PhaseType) bool {
	switch phase.Type {
	case Delivery:
		return phaseType != Delivery
	case Verification:
		return phaseType == Deploy
	case Deploy:
		return false
	}
	return false
}

func (phase *Phase) IsInActivePhaseGroup() bool {
	return phase.PhaseGroup.IsActivePhaseGroup()
}

func (phase *Phase) EarlierPhasesComplete() bool {
	switch phase.Type {
	case Delivery:
		return true
	case Verification:
		return phase.PhaseGroup.Delivery.IsComplete()
	case Deploy:
		return phase.PhaseGroup.Delivery.IsComplete() && phase.PhaseGroup.Verification.IsComplete()
	}
	return false
}

func (phaseGroup *PhaseGroup) IsActivePhaseGroup() bool {
	return phaseGroup.ID == phaseGroup.Train.ActivePhases.ID
}

func (phaseGroup *PhaseGroup) AddNewPhase(phaseType PhaseType, train *Train) *Phase {
	phase := &Phase{
		Type:       phaseType,
		Train:      train,
		PhaseGroup: phaseGroup,
	}
	switch phase.Type {
	case Delivery:
		phaseGroup.Delivery = phase
	case Verification:
		phaseGroup.Verification = phase
	case Deploy:
		phaseGroup.Deploy = phase
	}
	return phase
}

func (phaseGroup *PhaseGroup) SetReferences(train *Train) {
	phaseGroup.Train = train
	phaseGroup.Delivery.Train = train
	phaseGroup.Verification.Train = train
	phaseGroup.Deploy.Train = train

	phaseGroup.Delivery.PhaseGroup = phaseGroup
	phaseGroup.Verification.PhaseGroup = phaseGroup
	phaseGroup.Deploy.PhaseGroup = phaseGroup
}

func (phaseGroup *PhaseGroup) Phases() []*Phase {
	return []*Phase{
		phaseGroup.Delivery,
		phaseGroup.Verification,
		phaseGroup.Deploy,
	}
}

func (phaseGroup *PhaseGroup) GitReference() string {
	return fmt.Sprintf("%s-%s", phaseGroup.Train.Branch, ShortSHA(phaseGroup.HeadSHA))
}

type Jobs []*Job

func (jobs Jobs) CompletedNames() []string {
	names := make([]string, 0)
	if len(jobs) == 0 {
		return names
	}
	for _, job := range jobs {
		if job.Result == Ok && job.CompletedAt.HasValue() {
			names = append(names, job.Name)
		}
	}
	return names
}

func JobsForPhase(phaseType PhaseType) []string {
	var jobs []string
	var customJobs []string
	switch phaseType {
	case Delivery:
		jobs = settings.DeliveryJobs
		customJobs = settings.CustomDeliveryJobs
	case Verification:
		jobs = settings.VerificationJobs
		customJobs = settings.CustomVerificationJobs
	case Deploy:
		jobs = settings.DeployJobs
		customJobs = settings.CustomDeployJobs
	}

	if customJobs != nil {
		return customJobs
	}
	return jobs
}

// Should only be used for tests or fake implementation.
func CustomizeJobs(phaseType PhaseType, jobs []string) {
	switch phaseType {
	case Delivery:
		settings.CustomDeliveryJobs = jobs
	case Verification:
		settings.CustomVerificationJobs = jobs
	case Deploy:
		settings.CustomDeployJobs = jobs
	}
}

func (ticket *Ticket) IsComplete() bool {
	return ticket.ClosedAt.HasValue() || ticket.DeletedAt.HasValue()
}

// Sorting

type CommitsByID []*Commit

func (s CommitsByID) Len() int {
	return len(s)
}
func (s CommitsByID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s CommitsByID) Less(i, j int) bool {
	return s[i].ID < s[j].ID
}

type TicketsByID []*Ticket

func (s TicketsByID) Len() int {
	return len(s)
}
func (s TicketsByID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s TicketsByID) Less(i, j int) bool {
	return s[i].ID < s[j].ID
}

type JobsByID []*Job

func (s JobsByID) Len() int {
	return len(s)
}
func (s JobsByID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s JobsByID) Less(i, j int) bool {
	return s[i].ID < s[j].ID
}
