package guard

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Target struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
}
type Config struct {
	ActiveConfig     string   `json:"active_config"`
	Xray             string   `json:"xray"`
	OutboundTag      string   `json:"outbound_tag"`
	ActiveSOCKS      string   `json:"active_socks"`
	ProbePort        int      `json:"probe_port"`
	Restart          []string `json:"restart"`
	SubscriptionFile string   `json:"subscription_file"`
	StateDir         string   `json:"state_dir"`
	RuntimeDir       string   `json:"runtime_dir"`
	Include          string   `json:"include"`
	Interval         int      `json:"interval_seconds"`
	FailureThreshold int      `json:"failure_threshold"`
	Cooldown         int      `json:"cooldown_seconds"`
	RefreshInterval  int      `json:"refresh_seconds"`
	Timeout          int      `json:"timeout_seconds"`
	Targets          []Target `json:"targets"`
}

func LoadConfig(path string) (Config, error) {
	var c Config
	data, err := privateRead(path, 65536)
	if err != nil {
		return c, errors.New("cannot read private configuration (regular file, owner-only permissions required)")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil {
		return c, errors.New("invalid configuration JSON or unknown field")
	}
	if dec.Decode(new(any)) != io.EOF {
		return c, errors.New("configuration must contain one JSON object")
	}
	return c, c.Validate()
}

func (c *Config) Validate() error {
	for _, p := range []string{c.ActiveConfig, c.Xray, c.SubscriptionFile, c.StateDir, c.RuntimeDir} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return errors.New("file paths must be absolute and clean")
		}
	}
	if c.ActiveConfig == c.SubscriptionFile || filepath.Dir(c.ActiveConfig) == c.StateDir {
		return errors.New("active configuration must be separate from guard state and subscription")
	}
	if c.OutboundTag == "" || len(c.OutboundTag) > 128 {
		return errors.New("outbound_tag is required")
	}
	host, port, err := net.SplitHostPort(c.ActiveSOCKS)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || port == "" {
		return errors.New("active_socks must be a numeric loopback IP:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("invalid active SOCKS port")
	}
	if c.StateDir == c.RuntimeDir {
		return errors.New("persistent state and volatile runtime directories must be different")
	}
	if c.ProbePort < 1024 || c.ProbePort > 65535 || port == itoa(c.ProbePort) {
		return errors.New("probe_port must be a separate unprivileged TCP port")
	}
	if len(c.Restart) == 0 || !filepath.IsAbs(c.Restart[0]) {
		return errors.New("restart must be an argv array with an absolute executable")
	}
	if c.Interval == 0 {
		c.Interval = 30
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = 3
	}
	if c.Cooldown == 0 {
		c.Cooldown = 120
	}
	if c.RefreshInterval == 0 {
		c.RefreshInterval = 21600
	}
	if c.Timeout == 0 {
		c.Timeout = 8
	}
	if c.Interval < 5 || c.Interval > 3600 || c.FailureThreshold < 2 || c.FailureThreshold > 100 || c.Cooldown < 30 || c.Cooldown > 86400 || c.RefreshInterval < 60 || c.RefreshInterval > 604800 || c.Timeout < 1 || c.Timeout > 60 {
		return errors.New("timing values outside supported bounds")
	}
	if len(c.Targets) == 0 || len(c.Targets) > 8 {
		return errors.New("configure between one and eight health targets")
	}
	for _, t := range c.Targets {
		u, e := url.Parse(t.URL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || t.Status < 200 || t.Status > 299 {
			return errors.New("health targets require HTTPS and an explicit 2xx status")
		}
	}
	if _, e := regexp.Compile(c.Include); e != nil {
		return errors.New("invalid include expression")
	}
	return nil
}

func privateRead(path string, limit int64) ([]byte, error) {
	f, e := privateOpen(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > limit || !ownedByCurrentUser(st) {
		return nil, errors.New("unsafe or oversized file")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("file exceeds limit")
	}
	return b, e
}
