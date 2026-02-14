package domain

import "time"

const Version = 1

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
	VisibilityUnknown Visibility = "unknown"
)

type Operation string

const (
	OperationNone       Operation = "none"
	OperationMerge      Operation = "merge"
	OperationRebase     Operation = "rebase"
	OperationCherryPick Operation = "cherry-pick"
	OperationBisect     Operation = "bisect"
)

type UnsyncableReason string

const (
	ReasonMissingOrigin          UnsyncableReason = "missing_origin"
	ReasonOperationInProgress    UnsyncableReason = "operation_in_progress"
	ReasonDirtyTracked           UnsyncableReason = "dirty_tracked"
	ReasonDirtyUntracked         UnsyncableReason = "dirty_untracked"
	ReasonMissingUpstream        UnsyncableReason = "missing_upstream"
	ReasonDiverged               UnsyncableReason = "diverged"
	ReasonPushPolicyBlocked      UnsyncableReason = "push_policy_blocked"
	ReasonPushFailed             UnsyncableReason = "push_failed"
	ReasonPullFailed             UnsyncableReason = "pull_failed"
	ReasonCheckoutFailed         UnsyncableReason = "checkout_failed"
	ReasonTargetPathNonRepo      UnsyncableReason = "target_path_nonempty_not_repo"
	ReasonTargetPathRepoMismatch UnsyncableReason = "target_path_repo_mismatch"
)

type Catalog struct {
	Name                              string `yaml:"name"`
	Root                              string `yaml:"root"`
	RepoPathDepth                     int    `yaml:"repo_path_depth,omitempty"`
	AllowAutoPushDefaultBranchPrivate *bool  `yaml:"allow_auto_push_default_branch_private,omitempty"`
	AllowAutoPushDefaultBranchPublic  *bool  `yaml:"allow_auto_push_default_branch_public,omitempty"`
}

type ConfigFile struct {
	Version        int            `yaml:"version"`
	StateTransport StateTransport `yaml:"state_transport"`
	GitHub         GitHubConfig   `yaml:"github"`
	Sync           SyncConfig     `yaml:"sync"`
	Notify         NotifyConfig   `yaml:"notify"`
}

type StateTransport struct {
	Mode string `yaml:"mode"`
}

type GitHubConfig struct {
	Owner             string `yaml:"owner"`
	DefaultVisibility string `yaml:"default_visibility"`
	RemoteProtocol    string `yaml:"remote_protocol"`
}

type SyncConfig struct {
	AutoDiscover            bool `yaml:"auto_discover"`
	IncludeUntrackedAsDirty bool `yaml:"include_untracked_as_dirty"`
	DefaultAutoPushPrivate  bool `yaml:"default_auto_push_private"`
	DefaultAutoPushPublic   bool `yaml:"default_auto_push_public"`
	FetchPrune              bool `yaml:"fetch_prune"`
	PullFFOnly              bool `yaml:"pull_ff_only"`
	ScanFreshnessSeconds    int  `yaml:"scan_freshness_seconds"`
}

type NotifyConfig struct {
	Enabled         bool `yaml:"enabled"`
	Dedupe          bool `yaml:"dedupe"`
	ThrottleMinutes int  `yaml:"throttle_minutes"`
}

type RepoMetadataFile struct {
	Version             int        `yaml:"version"`
	RepoKey             string     `yaml:"repo_key"`
	Name                string     `yaml:"name"`
	OriginURL           string     `yaml:"origin_url"`
	Visibility          Visibility `yaml:"visibility"`
	PreferredCatalog    string     `yaml:"preferred_catalog"`
	PreferredRemote     string     `yaml:"preferred_remote"`
	AutoPush            bool       `yaml:"auto_push"`
	BranchFollowEnabled bool       `yaml:"branch_follow_enabled"`
}

type MachineFile struct {
	Version          int                 `yaml:"version"`
	MachineID        string              `yaml:"machine_id"`
	Hostname         string              `yaml:"hostname"`
	DefaultCatalog   string              `yaml:"default_catalog"`
	Catalogs         []Catalog           `yaml:"catalogs"`
	LastScanAt       time.Time           `yaml:"last_scan_at"`
	LastScanCatalogs []string            `yaml:"last_scan_catalogs,omitempty"`
	UpdatedAt        time.Time           `yaml:"updated_at"`
	Repos            []MachineRepoRecord `yaml:"repos"`
}

type MachineRepoRecord struct {
	RepoKey             string             `yaml:"repo_key"`
	Name                string             `yaml:"name"`
	Catalog             string             `yaml:"catalog"`
	Path                string             `yaml:"path"`
	OriginURL           string             `yaml:"origin_url"`
	Branch              string             `yaml:"branch"`
	HeadSHA             string             `yaml:"head_sha"`
	Upstream            string             `yaml:"upstream"`
	RemoteHeadSHA       string             `yaml:"remote_head_sha"`
	Ahead               int                `yaml:"ahead"`
	Behind              int                `yaml:"behind"`
	Diverged            bool               `yaml:"diverged"`
	HasDirtyTracked     bool               `yaml:"has_dirty_tracked"`
	HasUntracked        bool               `yaml:"has_untracked"`
	OperationInProgress Operation          `yaml:"operation_in_progress"`
	Syncable            bool               `yaml:"syncable"`
	UnsyncableReasons   []UnsyncableReason `yaml:"unsyncable_reasons"`
	StateHash           string             `yaml:"state_hash"`
	ObservedAt          time.Time          `yaml:"observed_at"`
}

type MachineRepoRecordWithMachine struct {
	MachineID string
	Record    MachineRepoRecord
}

type ObservedRepoState struct {
	OriginURL            string
	Branch               string
	HeadSHA              string
	Upstream             string
	RemoteHeadSHA        string
	Ahead                int
	Behind               int
	Diverged             bool
	HasDirtyTracked      bool
	HasUntracked         bool
	OperationInProgress  Operation
	IncludeUntrackedRule bool
}

type NotifyCacheFile struct {
	Version  int                         `yaml:"version"`
	LastSent map[string]NotifyCacheEntry `yaml:"last_sent"`
}

type NotifyCacheEntry struct {
	Fingerprint string    `yaml:"fingerprint"`
	SentAt      time.Time `yaml:"sent_at"`
}
