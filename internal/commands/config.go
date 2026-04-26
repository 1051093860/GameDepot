package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
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
		fmt.Println("project config:     " + root + string(os.PathSeparator) + config.ConfigRelPath)
	} else {
		fmt.Println("project config:     <not inside a GameDepot project>")
	}

	return nil
}

func ConfigProfiles(ctx context.Context) error {
	_ = ctx

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("default profile:", cfg.DefaultProfile)
	fmt.Println("profiles:")
	for _, name := range names {
		p := cfg.Profiles[name]
		mark := " "
		if name == cfg.DefaultProfile {
			mark = "*"
		}
		switch p.Type {
		case "local":
			fmt.Printf("%s %-16s %-8s %s\n", mark, name, p.Type, p.Path)
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

func ConfigUser(ctx context.Context, name string, email string) error {
	_ = ctx

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	changed := false
	if strings.TrimSpace(name) != "" {
		cfg.User.Name = strings.TrimSpace(name)
		changed = true
	}
	if strings.TrimSpace(email) != "" {
		cfg.User.Email = strings.TrimSpace(email)
		changed = true
	}

	if changed {
		if err := config.SaveGlobalConfig(cfg); err != nil {
			return err
		}
		fmt.Println("Saved global user config")
	}

	fmt.Printf("name: %s\n", cfg.User.Name)
	fmt.Printf("email: %s\n", cfg.User.Email)
	return nil
}
