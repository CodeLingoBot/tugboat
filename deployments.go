package tugboat

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ejholmes/hookshot/events"

	gorp "gopkg.in/gorp.v1"
)

const (
	// Maximum number of Deployments to return.
	DefaultDeploymentsLimit = 20
)

type DeployOpts struct {
	// This should be the github deployment id.
	ID int64

	// The git sha.
	Sha string

	// The git ref that was resolved to the above sha.
	Ref string

	// The environment to deploy to.
	Environment string

	// A description provided when the deployment was triggered.
	Description string

	// The repo that this deployment is for.
	Repo string

	// The name of the provider that this deployment relates to. In general,
	// this would be the platform that is being deployed to (e.g.
	// heroku/empire).
	Provider string
}

// NewDeployOptsFromWebhook instantiates a new DeployOpts instance based on the
// values inside a `deployment` event webhook payload.
func NewDeployOptsFromReader(r io.Reader) (DeployOpts, error) {
	var f events.Deployment

	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return DeployOpts{}, err
	}

	return DeployOpts{
		ID:          f.Deployment.ID,
		Sha:         f.Deployment.Sha,
		Ref:         f.Deployment.Ref,
		Environment: f.Deployment.Environment,
		Description: f.Deployment.Description,
		Repo:        f.Repository.FullName,
	}, nil
}

// StatusUpdate is used to update the status of a Deployment.
type StatusUpdate struct {
	Status DeploymentStatus
	Error  *error
}

// DeploymentStatus represents the status of a deployment.
type DeploymentStatus int

// The various states that a deployment can be in.
const (
	StatusPending DeploymentStatus = iota
	StatusStarted
	StatusFailed
	StatusErrored
	StatusSucceeded
)

func (s DeploymentStatus) String() string {
	switch s {
	case StatusFailed:
		return "failed"
	case StatusStarted:
		return "started"
	case StatusErrored:
		return "errored"
	case StatusSucceeded:
		return "succeeded"
	default:
		return "pending"
	}
}

func (s DeploymentStatus) IsCompleted() bool {
	if s == StatusFailed || s == StatusErrored || s == StatusSucceeded {
		return true
	}

	return false
}

// Scan implements the sql.Scanner interface.
func (s *DeploymentStatus) Scan(src interface{}) error {
	if src, ok := src.([]byte); ok {
		*s = newDeploymentStatus(string(src))
	}

	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (s *DeploymentStatus) UnmarshalJSON(b []byte) error {
	var src string

	if err := json.Unmarshal(b, &src); err != nil {
		return err
	}

	*s = newDeploymentStatus(src)

	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (s DeploymentStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func newDeploymentStatus(s string) DeploymentStatus {
	switch s {
	case "failed":
		return StatusFailed
	case "started":
		return StatusStarted
	case "errored":
		return StatusErrored
	case "succeeded":
		return StatusSucceeded
	default:
		return StatusPending
	}
}

// Value implements the driver.Value interface.
func (s DeploymentStatus) Value() (driver.Value, error) {
	return driver.Value(s.String()), nil
}

// Deployment represents a deployment.
type Deployment struct {
	// An internal ID for this Deployment.
	ID string `db:"id"`

	// The status of the deployment.
	Status DeploymentStatus `db:"status"`

	// The associated Deployment on github.
	GitHubID int64 `db:"github_id"`

	// The Sha that is being deployed.
	Sha string `db:"sha"`

	// The Ref that resolves to the Sha.
	Ref string `db:"ref"`

	// The environment that's being deployed to.
	Environment string `db:"environment"`

	// An optional description of the deployment.
	Description string `db:"description"`

	// The Repo that's being deployed to.
	Repo string `db:"repo"`

	// The name of the provider that was used to perform the deployment.
	Provider string `db:"provider"`

	// If the deployment failed, contains an error message.
	Error string `db:"error"`

	// The time that this deployment was created.
	CreatedAt time.Time `db:"created_at"`

	// The time that this deployment was started.
	StartedAt *time.Time `db:"started_at"`

	// The time that this deployment completed.
	CompletedAt *time.Time `db:"completed_at"`

	prevStatus DeploymentStatus `db:"-"`
}

// newDeployment returns a new Deployment instance based on the options.
func newDeployment(opts DeployOpts) *Deployment {
	return &Deployment{
		GitHubID:    opts.ID,
		Sha:         opts.Sha,
		Ref:         opts.Ref,
		Environment: opts.Environment,
		Description: opts.Description,
		Repo:        opts.Repo,
	}
}

// URL returns a url to view this deployment.
func (d *Deployment) URL() string {
	return fmt.Sprintf("%s/deploys/%s", BaseURL, d.ID)
}

// Started marks the Deployment as StatusStarted.
func (d *Deployment) Started(provider string) {
	t := time.Now()
	d.StartedAt = &t
	d.Status = StatusStarted
	d.Provider = provider
}

// Succeeded marks the Deployment as StatusSucceeded.
func (d *Deployment) Succeeded() {
	d.changeStatus(StatusSucceeded)
}

// Errored marks the deployment as errored. An error can be provided to show in UI
// for the reason it failed.
func (d *Deployment) Errored(err error) {
	d.Error = err.Error()
	d.changeStatus(StatusErrored)
}

// Failed marks the deployment as StatusFailed.
func (d *Deployment) Failed() {
	d.changeStatus(StatusFailed)
}

// PreInsert implements a pre insert hook for the db interface
func (d *Deployment) PreInsert(s gorp.SqlExecutor) error {
	d.CreatedAt = time.Now()
	return nil
}

// changeStatus changes the Status field to the provided status, and sets the
// CompletedAt field if the status represents a completed deployment.
func (d *Deployment) changeStatus(status DeploymentStatus) {
	if status.IsCompleted() {
		t := time.Now()
		d.CompletedAt = &t
	}
	d.prevStatus, d.Status = d.Status, status
}

// DeploymentsQuery is a query object for querying Deployments.
type DeploymentsQuery struct {
	Limit int
}

// DeploymentsCreate inserts a Deployment into the store.
func (s *store) DeploymentsCreate(d *Deployment) error {
	return s.db.Insert(d)
}

// DeploymentsUpdate inserts a Deployment into the store.
func (s *store) DeploymentsUpdate(d *Deployment) error {
	_, err := s.db.Update(d)
	return err
}

// Deployments returns the most recent Deployments.
func (s *store) Deployments(q DeploymentsQuery) ([]*Deployment, error) {
	var d []*Deployment

	limit := q.Limit
	if limit == 0 {
		limit = DefaultDeploymentsLimit
	}

	_, err := s.db.Select(&d, fmt.Sprintf(`select * from deployments order by github_id desc limit %d`, limit))
	return d, err
}

// DeploymentsFind finds a Deployment by id.
func (s *store) DeploymentsFind(id string) (*Deployment, error) {
	var d Deployment
	return &d, s.db.SelectOne(&d, `select * from deployments where id = $1`, id)
}

// deploymentsService wraps the DeploymentsCreate and DeploymentsUpdate methods.
type deploymentsService interface {
	DeploymentsCreate(*Deployment) error
	DeploymentsUpdate(*Deployment) error
}

// newDeploymentsService returns a new composed deploymentsService.
func newDeploymentsService(store *store, updater statusUpdater) deploymentsService {
	return &statusDeploymentsService{
		deploymentsService: store,
		updater:            updater,
	}
}

// statusDeploymentsService is a deploymentsService implementation that notifys
// a statusUpdater when the status of the deployment changes.
type statusDeploymentsService struct {
	deploymentsService

	updater statusUpdater
}

// DeploymentsUpdate notifies the updater if the status of the Deployment has
// changed, then it delegates the the wrapped deploymentsService.
func (s *statusDeploymentsService) DeploymentsUpdate(d *Deployment) error {
	if d.Status != d.prevStatus {
		if err := s.updater.UpdateStatus(d); err != nil {
			return err
		}
	}

	return s.deploymentsService.DeploymentsUpdate(d)
}

// DeploymentsCreate delegates to the wrapped deploymentsService then notifies
// the status updater about the deployment.
func (s *statusDeploymentsService) DeploymentsCreate(d *Deployment) error {
	if err := s.deploymentsService.DeploymentsCreate(d); err != nil {
		return err
	}

	return s.updater.UpdateStatus(d)
}

// deploymentChannel returns the name of the channel to send pusher events on
// for deployments.
func deploymentChannel(id string) string {
	return fmt.Sprintf("private-deployments-%s", id)
}
