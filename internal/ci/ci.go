package ci

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/go-github/v60/github"
)

// ============================================
// Data Models
// ============================================

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	HeadBranch  string    `json:"head_branch"`
	HeadSHA     string    `json:"head_sha"`
	Status      string    `json:"status"`       // queued, in_progress, completed
	Conclusion  string    `json:"conclusion"`   // success, failure, cancelled, skipped, etc.
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	RunNumber   int       `json:"run_number"`
	Event       string    `json:"event"`        // push, pull_request, etc.
	Actor       *Actor    `json:"actor"`
	Jobs        []Job     `json:"jobs,omitempty"`
	URL         string    `json:"html_url"`
}

type Actor struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type Job struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at"`
	Steps        []Step    `json:"steps,omitempty"`
	URL          string    `json:"html_url"`
}

type Step struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Number      int       `json:"number"`
}

// WorkflowFile represents a GitHub Actions workflow file (.yml)
type WorkflowFile struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	State       string   `json:"state"` // active, disabled
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	URL         string   `json:"html_url"`
	BadgeURL    string   `json:"badge_url"`
}

// RepositoryStatus represents the CI/CD status of a repository
type RepositoryStatus struct {
	Owner       string       `json:"owner"`
	Repo        string       `json:"repo"`
	DefaultBranch string     `json:"default_branch"`
	Workflows   []WorkflowFile `json:"workflows"`
	RecentRuns  []WorkflowRun  `json:"recent_runs"`
	OverallStatus string     `json:"overall_status"` // success, failure, in_progress, unknown
}

// CIService provides CI/CD integration with GitHub Actions
type CIService struct {
	client *github.Client
}

func NewCIService(client *github.Client) *CIService {
	return &CIService{client: client}
}

// SetClient updates the GitHub client
func (s *CIService) SetClient(client *github.Client) {
	s.client = client
}

// ============================================
// Workflow Management
// ============================================

// ListWorkflows lists all workflow files in a repository
func (s *CIService) ListWorkflows(ctx context.Context, owner, repo string) ([]WorkflowFile, error) {
	workflows, _, err := s.client.Actions.ListWorkflows(ctx, owner, repo, nil)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	var result []WorkflowFile
	for _, w := range workflows.Workflows {
		result = append(result, WorkflowFile{
			Name:        w.GetName(),
			Path:        w.GetPath(),
			State:       w.GetState(),
			CreatedAt:   w.GetCreatedAt().Format(time.RFC3339),
			UpdatedAt:   w.GetUpdatedAt().Format(time.RFC3339),
			URL:         w.GetHTMLURL(),
BadgeURL:    fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%d/badge.svg", owner, w.GetName(), w.GetID()),
		})
	}
	return result, nil
}

// GetWorkflow gets a specific workflow by ID
func (s *CIService) GetWorkflow(ctx context.Context, owner, repo string, workflowID int64) (*WorkflowFile, error) {
	w, _, err := s.client.Actions.GetWorkflowByID(ctx, owner, repo, workflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	return &WorkflowFile{
		Name:        w.GetName(),
		Path:        w.GetPath(),
		State:       w.GetState(),
		CreatedAt:   w.GetCreatedAt().Format(time.RFC3339),
		UpdatedAt:   w.GetUpdatedAt().Format(time.RFC3339),
		URL:         w.GetHTMLURL(),
		BadgeURL:    fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%d/badge.svg", owner, w.GetName(), w.GetID()),
	}, nil
}

// EnableWorkflow enables a workflow
func (s *CIService) EnableWorkflow(ctx context.Context, owner, repo string, workflowID int64) error {
	_, err := s.client.Actions.EnableWorkflowByID(ctx, owner, repo, workflowID)
	return err
}

// DisableWorkflow disables a workflow
func (s *CIService) DisableWorkflow(ctx context.Context, owner, repo string, workflowID int64) error {
	_, err := s.client.Actions.DisableWorkflowByID(ctx, owner, repo, workflowID)
	return err
}

// CreateWorkflowDispatch creates a workflow dispatch event
func (s *CIService) CreateWorkflowDispatch(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]interface{}) error {
	_, err := s.client.Actions.CreateWorkflowDispatchEventByID(ctx, owner, repo, workflowID, github.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: inputs,
	})
	return err
}

// ============================================
// Workflow Runs
// ============================================

// ListWorkflowRuns lists recent workflow runs
func (s *CIService) ListWorkflowRuns(ctx context.Context, owner, repo string, workflowID int64, opts *github.ListWorkflowRunsOptions) ([]WorkflowRun, error) {
	runs, _, err := s.client.Actions.ListWorkflowRunsByID(ctx, owner, repo, workflowID, opts)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}

	var result []WorkflowRun
	for _, run := range runs.WorkflowRuns {
		result = append(result, workflowRunFromGitHub(run))
	}
	return result, nil
}

// ListRepositoryWorkflowRuns lists all recent workflow runs for a repository
func (s *CIService) ListRepositoryWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) ([]WorkflowRun, error) {
	runs, _, err := s.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("list repository workflow runs: %w", err)
	}

	var result []WorkflowRun
	for _, run := range runs.WorkflowRuns {
		result = append(result, workflowRunFromGitHub(run))
	}
	return result, nil
}

// GetWorkflowRun gets a specific workflow run
func (s *CIService) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRun, error) {
	run, _, err := s.client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}
	runDetail := workflowRunFromGitHub(run)
	return &runDetail, nil
}

// ReRunWorkflow re-runs a workflow (creates a new dispatch)
// ReRunWorkflow re-runs a workflow (creates a new dispatch)
func (s *CIService) ReRunWorkflow(ctx context.Context, owner, repo string, runID int64) error {
	run, _, err := s.client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return fmt.Errorf("get workflow run: %w", err)
	}
	// Re-run by creating a new dispatch with the same ref
	_, err = s.client.Actions.CreateWorkflowDispatchEventByID(ctx, owner, repo, run.GetWorkflowID(), github.CreateWorkflowDispatchEventRequest{
		Ref: run.GetHeadBranch(),
	})
	return err
}

// CancelWorkflowRun cancels a workflow run
func (s *CIService) CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error {
	_, err := s.client.Actions.CancelWorkflowRunByID(ctx, owner, repo, runID)
	return err
}

// ListJobsForWorkflowRun lists jobs for a workflow run
func (s *CIService) ListJobsForWorkflowRun(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) ([]Job, error) {
	jobs, _, err := s.client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opts)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	var result []Job
	for _, job := range jobs.Jobs {
		result = append(result, Job{
			ID:          job.GetID(),
			Name:        job.GetName(),
			Status:      job.GetStatus(),
			Conclusion:  job.GetConclusion(),
			StartedAt:   job.GetStartedAt().Time,
			CompletedAt: job.GetCompletedAt().Time,
			URL:         job.GetHTMLURL(),
		})
	}
	return result, nil
}

// GetJobLogs gets logs for a job
func (s *CIService) GetJobLogs(ctx context.Context, owner, repo string, jobID int64, maxRedirects int) (string, error) {
	logURL, _, err := s.client.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, maxRedirects)
	if err != nil {
		return "", fmt.Errorf("get job logs: %w", err)
	}
	return logURL.String(), nil
}

// ============================================
// Repository CI/CD Status
// ============================================

// GetRepositoryStatus gets the overall CI/CD status for a repository
func (s *CIService) GetRepositoryStatus(ctx context.Context, owner, repo string) (*RepositoryStatus, error) {
	// Get workflows
	workflows, err := s.ListWorkflows(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	// Get recent runs
	runs, err := s.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{PerPage: 20},
	})
	if err != nil {
		return nil, err
	}

	// Get repo info
	repoObj, _, err := s.client.Repositories.Get(context.Background(), owner, repo)
	if err != nil {
		return nil, err
	}

	// Determine overall status
	overallStatus := "unknown"
	if len(runs) > 0 {
		latest := runs[0]
		if latest.Status == "in_progress" || latest.Status == "queued" {
			overallStatus = "in_progress"
		} else {
			overallStatus = latest.Conclusion
		}
	}

	// Convert workflows
	var workflowFiles []WorkflowFile
	for _, w := range workflows {
		workflowFiles = append(workflowFiles, WorkflowFile{
			Name:        w.Name,
			Path:        w.Path,
			State:       w.State,
			CreatedAt:   w.CreatedAt,
			UpdatedAt:   w.UpdatedAt,
			URL:         w.URL,
			BadgeURL:    w.BadgeURL,
		})
	}

	// Convert runs
	var recentRuns []WorkflowRun
	for _, r := range runs {
		recentRuns = append(recentRuns, WorkflowRun{
			ID:         r.ID,
			Name:       r.Name,
			HeadBranch: r.HeadBranch,
			HeadSHA:    r.HeadSHA,
			Status:     r.Status,
			Conclusion: r.Conclusion,
			CreatedAt:  r.CreatedAt,
			UpdatedAt:  r.UpdatedAt,
			RunNumber:  r.RunNumber,
			Event:      r.Event,
			URL:        r.URL,
		})
	}

	return &RepositoryStatus{
		Owner:         owner,
		Repo:          repo,
		DefaultBranch: repoObj.GetDefaultBranch(),
		Workflows:     workflowFiles,
		RecentRuns:    recentRuns,
		OverallStatus: overallStatus,
	}, nil
}

// ============================================
// Workflow File Management
// ============================================

// GetWorkflowFileContent gets the content of a workflow file
func (s *CIService) GetWorkflowFileContent(ctx context.Context, owner, repo, path string) (string, error) {
	content, _, _, err := s.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return "", fmt.Errorf("get workflow content: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(*content.Content)
	if err != nil {
		return "", fmt.Errorf("decode content: %w", err)
	}
	return string(decoded), nil
}

// ============================================
// Helper Functions
// ============================================

func workflowRunFromGitHub(run *github.WorkflowRun) WorkflowRun {
	return WorkflowRun{
		ID:          run.GetID(),
		Name:        run.GetName(),
		HeadBranch:  run.GetHeadBranch(),
		HeadSHA:     run.GetHeadSHA(),
		Status:      run.GetStatus(),
		Conclusion:  run.GetConclusion(),
		CreatedAt:   run.GetCreatedAt().Time,
		UpdatedAt:   run.GetUpdatedAt().Time,
		RunNumber:   run.GetRunNumber(),
		Event:       run.GetEvent(),
		Actor:       actorFromGitHub(run.GetActor()),
		URL:         run.GetHTMLURL(),
	}
}

func actorFromGitHub(actor *github.User) *Actor {
	if actor == nil {
		return nil
	}
	return &Actor{
		Login:     actor.GetLogin(),
		AvatarURL: actor.GetAvatarURL(),
	}
}
