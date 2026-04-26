package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AppDirName = "GameDepot"

type GlobalConfig struct {
	DefaultProfile string
	User           GlobalUser
	Profiles       map[string]StoreProfile
}

type GlobalUser struct {
	Name  string
	Email string
}

type StoreProfile struct {
	Type           string
	Path           string
	Endpoint       string
	Region         string
	Bucket         string
	ForcePathStyle bool
}

type CredentialsFile struct {
	Credentials map[string]Credentials
}

type Credentials struct {
	AccessKeyID     string
	AccessKeySecret string
}

func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		DefaultProfile: "local",
		User:           GlobalUser{},
		Profiles: map[string]StoreProfile{
			"local": {
				Type: "local",
				Path: ".gamedepot/remote_blobs",
			},
		},
	}
}

const EnvConfigDir = "GAMEDEPOT_CONFIG_DIR"

func GlobalDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvConfigDir)); override != "" {
		return override, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppDirName), nil
}

func GlobalConfigPath() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func GlobalCredentialsPath() (string, error) {
	dir, err := GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yaml"), nil
}

func LoadGlobalConfig() (GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return GlobalConfig{}, err
	}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return DefaultGlobalConfig(), nil
	}
	if err != nil {
		return GlobalConfig{}, err
	}
	defer f.Close()

	cfg := GlobalConfig{Profiles: map[string]StoreProfile{}}
	section := ""
	currentProfile := ""

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := leadingSpaces(raw)
		if indent == 0 && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			currentProfile = ""
			continue
		}

		if section == "profiles" && indent == 2 && strings.HasSuffix(line, ":") {
			currentProfile = strings.TrimSuffix(line, ":")
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]StoreProfile{}
			}
			if _, ok := cfg.Profiles[currentProfile]; !ok {
				cfg.Profiles[currentProfile] = StoreProfile{}
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = trimConfigValue(value)

		if section == "profiles" && currentProfile != "" {
			profile := cfg.Profiles[currentProfile]
			applyStoreProfileField(&profile, key, value)
			cfg.Profiles[currentProfile] = profile
			continue
		}

		if section == "user" {
			switch key {
			case "name":
				cfg.User.Name = value
			case "email":
				cfg.User.Email = value
			}
			continue
		}

		switch key {
		case "default_profile":
			cfg.DefaultProfile = value
		}
	}

	if err := scanner.Err(); err != nil {
		return GlobalConfig{}, err
	}

	ensureDefaultProfile(&cfg)
	return cfg, nil
}

func SaveGlobalConfig(cfg GlobalConfig) error {
	ensureDefaultProfile(&cfg)

	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "default_profile: %s\n\n", cfg.DefaultProfile)
	fmt.Fprintf(&b, "user:\n")
	fmt.Fprintf(&b, "  name: %s\n", cfg.User.Name)
	fmt.Fprintf(&b, "  email: %s\n\n", cfg.User.Email)
	fmt.Fprintf(&b, "profiles:\n")
	for name, p := range cfg.Profiles {
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    type: %s\n", p.Type)
		switch p.Type {
		case "local":
			fmt.Fprintf(&b, "    path: %s\n", p.Path)
		case "s3":
			fmt.Fprintf(&b, "    endpoint: %s\n", p.Endpoint)
			fmt.Fprintf(&b, "    region: %s\n", p.Region)
			fmt.Fprintf(&b, "    bucket: %s\n", p.Bucket)
			fmt.Fprintf(&b, "    force_path_style: %t\n", p.ForcePathStyle)
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func LoadCredentialsFile() (CredentialsFile, error) {
	path, err := GlobalCredentialsPath()
	if err != nil {
		return CredentialsFile{}, err
	}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return CredentialsFile{Credentials: map[string]Credentials{}}, nil
	}
	if err != nil {
		return CredentialsFile{}, err
	}
	defer f.Close()

	cf := CredentialsFile{Credentials: map[string]Credentials{}}
	section := ""
	currentProfile := ""

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := leadingSpaces(raw)
		if indent == 0 && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			currentProfile = ""
			continue
		}

		if section == "credentials" && indent == 2 && strings.HasSuffix(line, ":") {
			currentProfile = strings.TrimSuffix(line, ":")
			if _, ok := cf.Credentials[currentProfile]; !ok {
				cf.Credentials[currentProfile] = Credentials{}
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = trimConfigValue(value)

		if section == "credentials" && currentProfile != "" {
			cred := cf.Credentials[currentProfile]
			switch key {
			case "access_key_id":
				cred.AccessKeyID = value
			case "access_key_secret":
				cred.AccessKeySecret = value
			}
			cf.Credentials[currentProfile] = cred
		}
	}

	if err := scanner.Err(); err != nil {
		return CredentialsFile{}, err
	}

	return cf, nil
}

func SaveCredentialsFile(cf CredentialsFile) error {
	if cf.Credentials == nil {
		cf.Credentials = map[string]Credentials{}
	}

	path, err := GlobalCredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "credentials:\n")
	for name, c := range cf.Credentials {
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    access_key_id: %s\n", c.AccessKeyID)
		fmt.Fprintf(&b, "    access_key_secret: %s\n", c.AccessKeySecret)
	}

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func ResolveCredentials(profileName string) (Credentials, bool, error) {
	cf, err := LoadCredentialsFile()
	if err != nil {
		return Credentials{}, false, err
	}

	if c, ok := cf.Credentials[profileName]; ok && c.AccessKeyID != "" && c.AccessKeySecret != "" {
		return c, true, nil
	}

	id := os.Getenv("GAMEDEPOT_ACCESS_KEY_ID")
	secret := os.Getenv("GAMEDEPOT_ACCESS_KEY_SECRET")
	if id != "" && secret != "" {
		return Credentials{AccessKeyID: id, AccessKeySecret: secret}, true, nil
	}

	return Credentials{}, false, nil
}

func ensureDefaultProfile(cfg *GlobalConfig) {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]StoreProfile{}
	}
	if _, ok := cfg.Profiles["local"]; !ok {
		cfg.Profiles["local"] = StoreProfile{Type: "local", Path: ".gamedepot/remote_blobs"}
	}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "local"
	}
}

func applyStoreProfileField(profile *StoreProfile, key string, value string) {
	switch key {
	case "type":
		profile.Type = value
	case "path":
		profile.Path = value
	case "endpoint":
		profile.Endpoint = value
	case "region":
		profile.Region = value
	case "bucket":
		profile.Bucket = value
	case "force_path_style":
		profile.ForcePathStyle = parseBool(value)
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1", "on":
		return true
	default:
		return false
	}
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}
