package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ahmed/odoonoir/internal/github"
)

// ============================================
// Data Models
// ============================================

// MarketplaceModule represents a module in the marketplace
type MarketplaceModule struct {
	ID          string            `json:"id"`
	Owner       string            `json:"owner"`
	Repo        string            `json:"repo"`
	Name        string            `json:"name"`        // technical name from manifest
	DisplayName string            `json:"displayName"` // from manifest
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	License     string            `json:"license"`
	Category    string            `json:"category"`
	Version     string            `json:"version"`
	OdooVersions []string         `json:"odooVersions"`
	Stars       int               `json:"stars"`
	Forks       int               `json:"forks"`
	Topics      []string          `json:"topics"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	DefaultBranch string          `json:"defaultBranch"`
	IsPrivate   bool              `json:"isPrivate"`

	// Marketplace-specific fields
	Rating        float64   `json:"rating"`
	ReviewCount   int       `json:"reviewCount"`
	InstallCount  int       `json:"installCount"`
	LastIndexed   time.Time `json:"lastIndexed"`
	Verified      bool      `json:"verified"` // Official OCA/Odoo verified
	Featured      bool      `json:"featured"`
}

// Review represents a user review of a module
type Review struct {
	ID        string    `json:"id"`
	ModuleID  string    `json:"moduleId"`
	UserID    string    `json:"userId"`
	UserName  string    `json:"userName"`
	Rating    int       `json:"rating"`    // 1-5
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Helpful   int       `json:"helpful"`
}

// RatingSummary provides aggregate rating info
type RatingSummary struct {
	AverageRating float64 `json:"averageRating"`
	TotalReviews  int     `json:"totalReviews"`
	Distribution  [5]int  `json:"distribution"` // 1-5 stars
}

// MarketplaceStats provides marketplace-wide statistics
type MarketplaceStats struct {
	TotalModules    int     `json:"totalModules"`
	TotalReviews    int     `json:"totalReviews"`
	TotalInstalls   int     `json:"totalInstalls"`
	VerifiedModules int     `json:"verifiedModules"`
	Categories      []CategoryStat `json:"categories"`
}

type CategoryStat struct {
	Category  string `json:"category"`
	Count     int    `json:"count"`
	AvgRating float64 `json:"avgRating"`
}

// ============================================
// Storage
// ============================================

// Store interface for marketplace data persistence
type Store interface {
	// Modules
	GetModule(id string) (*MarketplaceModule, error)
	ListModules(filter ModuleFilter) ([]MarketplaceModule, error)
	SaveModule(module *MarketplaceModule) error
	DeleteModule(id string) error
	SearchModules(query string, filter ModuleFilter) ([]MarketplaceModule, error)

	// Reviews
	GetReviews(moduleID string) ([]Review, error)
	SaveReview(review *Review) error
	DeleteReview(reviewID string) error
	GetRatingSummary(moduleID string) (*RatingSummary, error)

	// Stats
	GetStats() (*MarketplaceStats, error)
}

type ModuleFilter struct {
	OdooVersion   string
	Category      string
	License       string
	MinRating     float64
	MinStars      int
	VerifiedOnly  bool
	FeaturedOnly  bool
	SortBy        string // "rating", "stars", "updated", "installs", "name"
	SortOrder     string // "asc", "desc"
	Page          int
	PerPage       int
}

// FileStore implements Store using JSON files
type FileStore struct {
	basePath string
	mu       sync.RWMutex
	modules  map[string]*MarketplaceModule
	reviews  map[string][]Review
}

func NewFileStore(basePath string) (*FileStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, err
	}
	store := &FileStore{
		basePath: basePath,
		modules:  make(map[string]*MarketplaceModule),
		reviews:  make(map[string][]Review),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileStore) load() error {
	modulesPath := filepath.Join(s.basePath, "modules.json")
	reviewsPath := filepath.Join(s.basePath, "reviews.json")

	if data, err := os.ReadFile(modulesPath); err == nil {
		var modules []MarketplaceModule
		if err := json.Unmarshal(data, &modules); err == nil {
			for _, m := range modules {
				s.modules[m.ID] = &m
			}
		}
	}

	if data, err := os.ReadFile(reviewsPath); err == nil {
		var reviews []Review
		if err := json.Unmarshal(data, &reviews); err == nil {
			for _, r := range reviews {
				s.reviews[r.ModuleID] = append(s.reviews[r.ModuleID], r)
			}
		}
	}
	return nil
}

func (s *FileStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modulesPath := filepath.Join(s.basePath, "modules.json")
	reviewsPath := filepath.Join(s.basePath, "reviews.json")

	modules := make([]MarketplaceModule, 0, len(s.modules))
	for _, m := range s.modules {
		modules = append(modules, *m)
	}
	data, _ := json.MarshalIndent(modules, "", "  ")
	if err := os.WriteFile(modulesPath, data, 0o644); err != nil {
		return err
	}

	reviews := make([]Review, 0)
	for _, rs := range s.reviews {
		reviews = append(reviews, rs...)
	}
	data, _ = json.MarshalIndent(reviews, "", "  ")
	return os.WriteFile(reviewsPath, data, 0o644)
}

// Module operations
func (s *FileStore) GetModule(id string) (*MarketplaceModule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.modules[id]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", id)
	}
	return m, nil
}

func (s *FileStore) ListModules(filter ModuleFilter) ([]MarketplaceModule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modules := make([]MarketplaceModule, 0, len(s.modules))
	for _, m := range s.modules {
		if s.matchesFilter(m, filter) {
			modules = append(modules, *m)
		}
	}

	sortModules(modules, filter.SortBy, filter.SortOrder)

	// Pagination
	start := filter.Page * filter.PerPage
	end := start + filter.PerPage
	if start >= len(modules) {
		return []MarketplaceModule{}, nil
	}
	if end > len(modules) {
		end = len(modules)
	}
	return modules[start:end], nil
}

func (s *FileStore) matchesFilter(m *MarketplaceModule, filter ModuleFilter) bool {
	if filter.OdooVersion != "" && !contains(m.OdooVersions, filter.OdooVersion) {
		return false
	}
	if filter.Category != "" && m.Category != filter.Category {
		return false
	}
	if filter.License != "" && m.License != filter.License {
		return false
	}
	if filter.MinRating > 0 && m.Rating < filter.MinRating {
		return false
	}
	if filter.MinStars > 0 && m.Stars < filter.MinStars {
		return false
	}
	if filter.VerifiedOnly && !m.Verified {
		return false
	}
	if filter.FeaturedOnly && !m.Featured {
		return false
	}
	return true
}

func (s *FileStore) SaveModule(module *MarketplaceModule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if module.ID == "" {
		module.ID = fmt.Sprintf("%s/%s", module.Owner, module.Repo)
	}
	module.LastIndexed = time.Now()
	s.modules[module.ID] = module
	return s.save()
}

func (s *FileStore) DeleteModule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.modules, id)
	delete(s.reviews, id)
	return s.save()
}

func (s *FileStore) SearchModules(query string, filter ModuleFilter) ([]MarketplaceModule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modules := make([]MarketplaceModule, 0)
	q := strings.ToLower(query)
	for _, m := range s.modules {
		if s.matchesFilter(m, filter) {
			if q == "" ||
				strings.Contains(strings.ToLower(m.Name), q) ||
				strings.Contains(strings.ToLower(m.DisplayName), q) ||
				strings.Contains(strings.ToLower(m.Description), q) ||
				strings.Contains(strings.ToLower(m.Owner), q) ||
				strings.Contains(strings.ToLower(m.Repo), q) {
				modules = append(modules, *m)
			}
		}
	}
	sortModules(modules, filter.SortBy, filter.SortOrder)

	start := filter.Page * filter.PerPage
	end := start + filter.PerPage
	if start >= len(modules) {
		return []MarketplaceModule{}, nil
	}
	if end > len(modules) {
		end = len(modules)
	}
	return modules[start:end], nil
}

func sortModules(modules []MarketplaceModule, sortBy, sortOrder string) {
	sort.Slice(modules, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "rating":
			less = modules[i].Rating < modules[j].Rating
		case "stars":
			less = modules[i].Stars < modules[j].Stars
		case "updated":
			less = modules[i].UpdatedAt.Before(modules[j].UpdatedAt)
		case "installs":
			less = modules[i].InstallCount < modules[j].InstallCount
		case "name":
			less = modules[i].Name < modules[j].Name
		default:
			less = modules[i].Rating < modules[j].Rating
		}
		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}

// Review operations
func (s *FileStore) GetReviews(moduleID string) ([]Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reviews[moduleID], nil
}

func (s *FileStore) SaveReview(review *Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if review.ID == "" {
		review.ID = fmt.Sprintf("review-%d", time.Now().UnixNano())
	}
	review.CreatedAt = time.Now()
	review.UpdatedAt = time.Now()
	s.reviews[review.ModuleID] = append(s.reviews[review.ModuleID], *review)

	// Update module rating
	if module, ok := s.modules[review.ModuleID]; ok {
		s.updateModuleRating(module)
	}
	return s.save()
}

func (s *FileStore) DeleteReview(reviewID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for moduleID, reviews := range s.reviews {
		for i, r := range reviews {
			if r.ID == reviewID {
				s.reviews[moduleID] = append(reviews[:i], reviews[i+1:]...)
				if module, ok := s.modules[moduleID]; ok {
					s.updateModuleRating(module)
				}
				return s.save()
			}
		}
	}
	return fmt.Errorf("review not found: %s", reviewID)
}

func (s *FileStore) GetRatingSummary(moduleID string) (*RatingSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reviews := s.reviews[moduleID]
	if len(reviews) == 0 {
		return &RatingSummary{}, nil
	}

	var sum float64
	dist := [5]int{}
	for _, r := range reviews {
		sum += float64(r.Rating)
		if r.Rating >= 1 && r.Rating <= 5 {
			dist[r.Rating-1]++
		}
	}
	return &RatingSummary{
		AverageRating: sum / float64(len(reviews)),
		TotalReviews:  len(reviews),
		Distribution:  dist,
	}, nil
}

func (s *FileStore) updateModuleRating(module *MarketplaceModule) {
	summary, _ := s.GetRatingSummary(module.ID)
	module.Rating = summary.AverageRating
	module.ReviewCount = summary.TotalReviews
}

func (s *FileStore) GetStats() (*MarketplaceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &MarketplaceStats{
		TotalModules:    len(s.modules),
		TotalReviews:    0,
		TotalInstalls:   0,
		VerifiedModules: 0,
	}

	categoryMap := make(map[string]CategoryStat)
	for _, m := range s.modules {
		stats.TotalInstalls += m.InstallCount
		if m.Verified {
			stats.VerifiedModules++
		}
		stats.TotalReviews += m.ReviewCount

		cat := categoryMap[m.Category]
		cat.Category = m.Category
		cat.Count++
		cat.AvgRating = (cat.AvgRating*float64(cat.Count-1) + m.Rating) / float64(cat.Count)
		categoryMap[m.Category] = cat
	}

	for _, cat := range categoryMap {
		stats.Categories = append(stats.Categories, cat)
	}
	sort.Slice(stats.Categories, func(i, j int) bool {
		return stats.Categories[i].Count > stats.Categories[j].Count
	})
	return stats, nil
}

// ============================================
// Marketplace Service
// ============================================

// Service provides high-level marketplace operations
type Service struct {
	store     Store
	github    *github.Client
	indexer   *Indexer
	mu        sync.RWMutex
	indexed   map[string]time.Time
}

func NewService(store Store, githubClient *github.Client) *Service {
	return &Service{
		store:   store,
		github:  githubClient,
		indexed: make(map[string]time.Time),
	}
}

// SetGitHubClient updates the GitHub client
func (s *Service) SetGitHubClient(githubClient *github.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.github = githubClient
}

// IndexModule fetches module info from GitHub and saves to marketplace
func (s *Service) IndexModule(ctx context.Context, owner, repo string) (*MarketplaceModule, error) {
	if s.github == nil {
		return nil, fmt.Errorf("GitHub client not configured")
	}

	detail, err := s.github.GetModule(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	module := &MarketplaceModule{
		ID:          fmt.Sprintf("%s/%s", owner, repo),
		Owner:       owner,
		Repo:        repo,
		Name:        detail.Manifest.Name,
		DisplayName: detail.Module.DisplayName,
		Summary:     detail.Manifest.Summary,
		Description: detail.Manifest.Description,
		Author:      detail.Manifest.Author,
		License:     detail.Module.License,
		Category:    detail.Manifest.Category,
		Version:     detail.Manifest.Version,
		OdooVersions: detail.Module.OdooVersions,
		Stars:       detail.Module.Stars,
		Forks:       detail.Module.Forks,
		Topics:      detail.Module.Topics,
		UpdatedAt:   parseTime(detail.Module.UpdatedAt),
		DefaultBranch: detail.Module.DefaultBranch,
		IsPrivate:   detail.Module.IsPrivate,
		Rating:      0,
		ReviewCount: 0,
		InstallCount: 0,
		Verified:    false,
		Featured:    false,
	}

	if err := s.store.SaveModule(module); err != nil {
		return nil, err
	}

	s.indexed[module.ID] = time.Now()
	return module, nil
}

// IndexAllModules indexes all modules from a GitHub organization
func (s *Service) IndexAllModules(ctx context.Context, owner string) ([]MarketplaceModule, error) {
	// Search for all Odoo modules from the organization
	query := github.SearchQuery{
		Query:       fmt.Sprintf("org:%s topic:odoo", owner),
		PerPage:     100,
		Sort:        "stars",
		Order:       "desc",
	}

	result, err := s.github.SearchModules(ctx, query)
	if err != nil {
		return nil, err
	}

	var modules []MarketplaceModule
	for _, repo := range result.Modules {
		mod, err := s.IndexModule(ctx, repo.Owner, repo.Repo)
		if err != nil {
			continue // Skip failed modules
		}
		modules = append(modules, *mod)
	}
	return modules, nil
}

// GetModule retrieves a module from the marketplace
func (s *Service) GetModule(id string) (*MarketplaceModule, error) {
	return s.store.GetModule(id)
}

// ListModules lists modules with filtering
func (s *Service) ListModules(filter ModuleFilter) ([]MarketplaceModule, error) {
	return s.store.ListModules(filter)
}

// SearchModules searches for modules
func (s *Service) SearchModules(query string, filter ModuleFilter) ([]MarketplaceModule, error) {
	return s.store.SearchModules(query, filter)
}

// GetModuleReviews returns reviews for a module
func (s *Service) GetModuleReviews(moduleID string) ([]Review, error) {
	return s.store.GetReviews(moduleID)
}

// AddReview adds a review to a module
func (s *Service) AddReview(review *Review) error {
	return s.store.SaveReview(review)
}

// GetModuleRating returns rating summary for a module
func (s *Service) GetModuleRating(moduleID string) (*RatingSummary, error) {
	return s.store.GetRatingSummary(moduleID)
}

// GetMarketplaceStats returns marketplace statistics
func (s *Service) GetStats() (*MarketplaceStats, error) {
	return s.store.GetStats()
}

// GetFeaturedModules returns featured modules
func (s *Service) GetFeaturedModules(limit int) ([]MarketplaceModule, error) {
	filter := ModuleFilter{
		FeaturedOnly: true,
		PerPage:      limit,
		SortBy:       "rating",
		SortOrder:    "desc",
	}
	return s.store.ListModules(filter)
}

// GetVerifiedModules returns verified modules
func (s *Service) GetVerifiedModules(limit int) ([]MarketplaceModule, error) {
	filter := ModuleFilter{
		VerifiedOnly: true,
		PerPage:      limit,
		SortBy:       "rating",
		SortOrder:    "desc",
	}
	return s.store.ListModules(filter)
}

// GetTopRatedModules returns top-rated modules
func (s *Service) GetTopRatedModules(limit int) ([]MarketplaceModule, error) {
	filter := ModuleFilter{
		MinRating: 4.0,
		PerPage:    limit,
		SortBy:     "rating",
		SortOrder:  "desc",
	}
	return s.store.ListModules(filter)
}

// GetMostInstalledModules returns most installed modules
func (s *Service) GetMostInstalledModules(limit int) ([]MarketplaceModule, error) {
	filter := ModuleFilter{
		PerPage:   limit,
		SortBy:    "installs",
		SortOrder: "desc",
	}
	return s.store.ListModules(filter)
}

// IncrementInstallCount increments the install count for a module
func (s *Service) IncrementInstallCount(moduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	module, err := s.store.GetModule(moduleID)
	if err != nil {
		return err
	}
	module.InstallCount++
	return s.store.SaveModule(module)
}

// ReindexModule re-indexes a module from GitHub
func (s *Service) ReindexModule(ctx context.Context, owner, repo string) (*MarketplaceModule, error) {
	// Remove old index
	moduleID := fmt.Sprintf("%s/%s", owner, repo)
	s.store.DeleteModule(moduleID)
	delete(s.indexed, moduleID)
	return s.IndexModule(ctx, owner, repo)
}

// ============================================
// Indexer - Background indexing service
// ============================================

type Indexer struct {
	service   *Service
	interval  time.Duration
	stopChan  chan struct{}
	mu        sync.Mutex
	running   bool
}

func NewIndexer(service *Service, interval time.Duration) *Indexer {
	return &Indexer{
		service:  service,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (i *Indexer) Start() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.running {
		return
	}
	i.running = true
	go i.run()
}

func (i *Indexer) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.running {
		return
	}
	close(i.stopChan)
	i.running = false
}

func (i *Indexer) run() {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.reindexStale()
		case <-i.stopChan:
			return
		}
	}
}

func (i *Indexer) reindexStale() {
	// Implementation would check for modules that haven't been indexed recently
	// and re-index them
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func parseTime(s string) time.Time {
	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try parsing as "2006-01-02"
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t
	}
	// Return zero time if parsing fails
	return time.Time{}
}
