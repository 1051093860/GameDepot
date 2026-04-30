package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
)

func ConfigPath(ctx context.Context, start string) error {
	_ = ctx

	gcfg, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}
	cred, err := config.GlobalCredentialsPath()
	if err != nil {
		return err
	}

	fmt.Println("global config:      " + gcfg)
	fmt.Println("global credentials: " + cred)

	if root, err := config.FindRoot(start); err == nil {
		fmt.Println("project root:       " + root)
		fmt.Println("project config:     " + root + string(os.PathSeparator) + config.ConfigRelPath)
	} else if candidate, candErr := config.FindProjectRootCandidate(start); candErr == nil {
		fmt.Println("project root:       " + candidate.Root + " (detected by " + candidate.Marker + ")")
		fmt.Println("project config:     " + candidate.Root + string(os.PathSeparator) + config.ConfigRelPath + " (missing)")
		fmt.Println("project status:     not initialized; run `gamedepot init .` here")
	} else {
		fmt.Println("project root:       <not inside a GameDepot or Unreal project>")
		fmt.Println("project config:     <not found>")
	}

	return nil
}

func ConfigProfiles(ctx context.Context) error {
	_ = ctx

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	projectRoot := ""
	if root, err := config.FindRoot("."); err == nil {
		projectRoot = root
	} else if candidate, candErr := config.FindProjectRootCandidate("."); candErr == nil {
		projectRoot = candidate.Root
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("default profile:", cfg.DefaultProfile)
	if projectRoot != "" {
		fmt.Println("project root:   ", projectRoot)
	}
	fmt.Println("profiles:")
	for _, name := range names {
		p := cfg.Profiles[name]
		mark := " "
		if name == cfg.DefaultProfile {
			mark = "*"
		}
		switch p.Type {
		case "local":
			path := p.Path
			if projectRoot != "" && !filepath.IsAbs(path) {
				path = path + " -> " + filepath.Join(projectRoot, filepath.FromSlash(path))
			}
			fmt.Printf("%s %-16s %-8s %s\n", mark, name, p.Type, path)
		case "s3":
			style := ""
			if p.ForcePathStyle {
				style = " path-style"
			}
			fmt.Printf("%s %-16s %-8s %s / %s%s\n", mark, name, p.Type, p.Endpoint, p.Bucket, style)
		default:
			fmt.Printf("%s %-16s %-8s\n", mark, name, p.Type)
		}
	}

	return nil
}

func ConfigAddLocal(ctx context.Context, name string, path string) error {
	_ = ctx

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if path == "" {
		path = ".gamedepot/remote_blobs"
	}

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	cfg.Profiles[name] = config.StoreProfile{
		Type: "local",
		Path: path,
	}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = name
	}

	if err := config.SaveGlobalConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Saved local profile %q\n", name)
	return nil
}

func ConfigAddS3(ctx context.Context, name string, endpoint string, region string, bucket string, forcePathStyle bool) error {
	_ = ctx

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if endpoint == "" {
		return fmt.Errorf("--endpoint is required")
	}
	if region == "" {
		region = "us-east-1"
	}
	if bucket == "" {
		return fmt.Errorf("--bucket is required")
	}

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	cfg.Profiles[name] = config.StoreProfile{
		Type:           "s3",
		Endpoint:       endpoint,
		Region:         region,
		Bucket:         bucket,
		ForcePathStyle: forcePathStyle,
	}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = name
	}

	if err := config.SaveGlobalConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Saved s3 profile %q\n", name)
	return nil
}

func ConfigAddOSS(ctx context.Context, name string, region string, bucket string, endpoint string, internal bool, accelerate bool) error {
	if region == "" {
		region = "cn-hangzhou"
	}
	if bucket == "" {
		return fmt.Errorf("--bucket is required")
	}
	if endpoint == "" {
		switch {
		case accelerate:
			endpoint = "https://s3.oss-accelerate.aliyuncs.com"
		case internal:
			endpoint = "https://s3.oss-" + region + "-internal.aliyuncs.com"
		default:
			endpoint = "https://s3.oss-" + region + ".aliyuncs.com"
		}
	}
	return ConfigAddS3(ctx, name, endpoint, region, bucket, false)
}

func ConfigSetCredentials(ctx context.Context, name string, accessKeyID string, accessKeySecret string) error {
	_ = ctx

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	if accessKeyID == "" || accessKeySecret == "" {
		id, secret, err := readCredentialsInteractively(accessKeyID, accessKeySecret)
		if err != nil {
			return err
		}
		accessKeyID = id
		accessKeySecret = secret
	}
	if accessKeyID == "" || accessKeySecret == "" {
		return fmt.Errorf("access key id and secret are required")
	}

	cf, err := config.LoadCredentialsFile()
	if err != nil {
		return err
	}
	if cf.Credentials == nil {
		cf.Credentials = map[string]config.Credentials{}
	}
	cf.Credentials[name] = config.Credentials{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
	}

	if err := config.SaveCredentialsFile(cf); err != nil {
		return err
	}

	fmt.Printf("Saved credentials for profile %q\n", name)
	return nil
}

func ConfigUse(ctx context.Context, name string) error {
	_ = ctx

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	cfg.DefaultProfile = name
	if err := config.SaveGlobalConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("Default profile: %s\n", name)
	return nil
}

func ConfigProjectUse(ctx context.Context, start string, name string) error {
	_ = ctx

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}

	global, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if _, ok := global.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found in global config", name)
	}

	root, err := config.FindRoot(start)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	cfg.Store.Profile = name
	if cfg.Store.Prefix == "" {
		cfg.Store.Prefix = config.DefaultStorePrefix(cfg.ProjectID)
	}
	// Clear legacy fields when writing v0.4 project config.
	cfg.Store.Type = ""
	cfg.Store.Root = ""

	if err := config.Save(root, cfg); err != nil {
		return err
	}

	fmt.Printf("Project now uses store profile %q\n", name)
	fmt.Printf("Prefix: %s\n", cfg.Store.Prefix)
	return nil
}

func readCredentialsInteractively(existingID string, existingSecret string) (string, string, error) {
	reader := bufio.NewReader(os.Stdin)

	id := existingID
	secret := existingSecret

	if id == "" {
		fmt.Print("Access Key ID: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		id = strings.TrimSpace(line)
	}
	if secret == "" {
		fmt.Print("Access Key Secret: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		secret = strings.TrimSpace(line)
	}

	return id, secret, nil
}

func ConfigUser(ctx context.Context, name string, email string, identity string) error {
	_ = ctx

	global, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	globalChanged := false
	if strings.TrimSpace(name) != "" {
		global.User.Name = strings.TrimSpace(name)
		globalChanged = true
	}
	if strings.TrimSpace(email) != "" {
		global.User.Email = strings.TrimSpace(email)
		globalChanged = true
	}

	if globalChanged {
		if err := config.SaveGlobalConfig(global); err != nil {
			return err
		}
		fmt.Println("Saved global user config")
	}

	projectRoot := ""
	if root, err := config.FindRoot("."); err == nil {
		projectRoot = root
	} else if strings.TrimSpace(identity) != "" {
		return fmt.Errorf("project user identity is no longer used")
	}

	gitLocalName := gitConfigValue("user.name", true)
	gitLocalEmail := gitConfigValue("user.email", true)
	gitGlobalName := gitConfigValue("user.name", false)
	gitGlobalEmail := gitConfigValue("user.email", false)

	effectiveName, nameSource := firstNonEmptyWithSource(
		global.User.Name, "global GameDepot",
		gitLocalName, "local Git",
		gitGlobalName, "global Git",
	)
	effectiveEmail, emailSource := firstNonEmptyWithSource(
		global.User.Email, "global GameDepot",
		gitLocalEmail, "local Git",
		gitGlobalEmail, "global Git",
	)

	if projectRoot != "" {
		fmt.Printf("project root:      %s\n", projectRoot)
	}
	fmt.Printf("name:              %s\n", effectiveName)
	fmt.Printf("email:             %s\n", effectiveEmail)
	fmt.Printf("name source:       %s\n", emptyLabel(nameSource))
	fmt.Printf("email source:      %s\n", emptyLabel(emailSource))
	fmt.Printf("global name:       %s\n", global.User.Name)
	fmt.Printf("global email:      %s\n", global.User.Email)
	fmt.Printf("git local name:    %s\n", gitLocalName)
	fmt.Printf("git local email:   %s\n", gitLocalEmail)
	fmt.Printf("git global name:   %s\n", gitGlobalName)
	fmt.Printf("git global email:  %s\n", gitGlobalEmail)
	return nil
}

func gitConfigValue(key string, local bool) string {
	args := []string{"config"}
	if !local {
		args = append(args, "--global")
	}
	args = append(args, "--get", key)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstNonEmptyWithSource(valuesAndSources ...string) (string, string) {
	for i := 0; i+1 < len(valuesAndSources); i += 2 {
		value := strings.TrimSpace(valuesAndSources[i])
		if value != "" {
			return value, valuesAndSources[i+1]
		}
	}
	return "", ""
}

func emptyLabel(s string) string {
	if strings.TrimSpace(s) == "" {
		return "<unset>"
	}
	return s
}
