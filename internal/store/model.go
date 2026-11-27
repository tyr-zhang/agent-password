// Package store owns the encrypted vault file: its data model, atomic writes
// and backup rotation. It never logs or returns secret material to callers
// outside the unlocked daemon.
package store

import "time"

type Kind string

const (
	KindWeb    Kind = "web"
	KindAPIKey Kind = "apikey"
	KindServer Kind = "server"
	KindEnvVar Kind = "envvar"
)

func ValidKind(k Kind) bool {
	switch k {
	case KindWeb, KindAPIKey, KindServer, KindEnvVar:
		return true
	default:
		return false
	}
}

// Secret holds the fields for all four credential kinds. Only one shape is
// populated per kind; the whole struct lives inside the encrypted body.
type Secret struct {
	// web
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TOTPSeed string `json:"totp_seed,omitempty"`
	URL      string `json:"url,omitempty"`

	// apikey
	BaseURL   string `json:"base_url,omitempty"`
	HeaderName string `json:"header_name,omitempty"`
	Token     string `json:"token,omitempty"`

	// server
	Protocol   string `json:"protocol,omitempty"` // ssh | mysql | postgres | redis
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	ServerUser string `json:"server_user,omitempty"`
	KeyPath    string `json:"key_path,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Database   string `json:"database,omitempty"`

	// environment variable
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// Credential metadata is safe to show; Secret is not.
type Credential struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	Label     string    `json:"label"`
	RiskLevel string    `json:"risk_level"` // normal | high
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Secret    Secret    `json:"secret"`
}

// PublicCredential is the only credential representation allowed to reach a
// CLI response. It deliberately has no Secret field.
type PublicCredential struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	Label     string    `json:"label"`
	RiskLevel string    `json:"risk_level"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Public(c *Credential) PublicCredential {
	return PublicCredential{
		ID: c.ID, Kind: c.Kind, Label: c.Label, RiskLevel: c.RiskLevel,
		Tags: c.Tags, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// Rule is an ordered policy rule. First match wins.
type Rule struct {
	ID        string `json:"id"`
	CredPat   string `json:"cred_pattern"`
	Action    string `json:"action"`
	Condition string `json:"condition,omitempty"` // "", sql_readonly, http_readonly
	Decision  string `json:"decision"`            // allow | confirm | deny
}

// Body is the plaintext vault body, encrypted as a single AES-GCM blob.
type Body struct {
	Schema      int          `json:"schema"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Credentials []Credential `json:"credentials"`
	Policies    []Rule       `json:"policies"`
}
