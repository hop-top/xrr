package redis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	xrr "hop.top/xrr"
	"gopkg.in/yaml.v3"
)

// Request represents a Redis interaction request.
type Request struct {
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
}

func (r *Request) AdapterID() string { return "redis" }

// Response represents a Redis interaction response.
type Response struct {
	Result any `yaml:"result"`
}

func (r *Response) AdapterID() string { return "redis" }

// Adapter implements xrr.Adapter for Redis interactions.
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string { return "redis" }

// Fingerprint: sha256(command + args joined)[:8].
func (a *Adapter) Fingerprint(req xrr.Request) (string, error) {
	r, ok := req.(*Request)
	if !ok {
		return "", fmt.Errorf("redis: unexpected request type %T", req)
	}
	parts := append([]string{strings.ToUpper(r.Command)}, r.Args...)
	canonical, err := json.Marshal(strings.Join(parts, " "))
	if err != nil {
		return "", fmt.Errorf("redis: fingerprint marshal: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", sum[:4]), nil
}

func (a *Adapter) Serialize(v any) ([]byte, error)          { return yaml.Marshal(v) }
func (a *Adapter) Deserialize(data []byte, target any) error { return yaml.Unmarshal(data, target) }
