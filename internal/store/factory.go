package store

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
)

type Info struct {
	Profile        string
	Type           string
	Path           string
	Endpoint       string
	Region         string
	Bucket         string
	Prefix         string
	ForcePathStyle bool
	Credentials    string
}

func NewFromProject(root string, project config.Config) (BlobStore, Info, error) {
	profileName := project.Store.Profile
	if profileName == "" && project.Store.Type != "" {
		return newLegacyStore(root, project)
	}

	global, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, Info{}, err
	}

	if profileName == "" {
		profileName = global.DefaultProfile
	}
	if profileName == "" {
		profileName = "local"
	}

	prefix := project.Store.Prefix
	if prefix == "" {
		prefix = config.DefaultStorePrefix(project.ProjectID)
	}

	profile, ok := global.Profiles[profileName]
	if !ok {
		path := filepath.Join(root, filepath.FromSlash(".gamedepot/remote_blobs"))
		return NewLocalBlobStore(path), Info{Profile: "local", Type: "local", Path: path, Prefix: prefix}, nil
	}
	if profile.Type == "" {
		profile.Type = "local"
	}

	switch profile.Type {
	case "local":
		path := profile.Path
		if path == "" {
			path = ".gamedepot/remote_blobs"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		return NewLocalBlobStore(path), Info{
			Profile: profileName,
			Type:    "local",
			Path:    path,
			Prefix:  prefix,
		}, nil

	case "s3":
		if strings.TrimSpace(profile.Bucket) == "" {
			path := filepath.Join(root, filepath.FromSlash(".gamedepot/remote_blobs"))
			return NewLocalBlobStore(path), Info{Profile: profileName, Type: "local", Path: path, Prefix: prefix}, nil
		}
		cred, ok, err := config.ResolveCredentials(profileName)
		if err != nil {
			return nil, Info{}, err
		}
		if !ok {
			return nil, Info{}, fmt.Errorf("missing credentials for profile %q; run `gamedepot config set-credentials %s` or set GAMEDEPOT_ACCESS_KEY_ID/GAMEDEPOT_ACCESS_KEY_SECRET", profileName, profileName)
		}
		s, err := NewS3BlobStore(S3Options{
			Endpoint:        profile.Endpoint,
			Region:          profile.Region,
			Bucket:          profile.Bucket,
			Prefix:          prefix,
			ForcePathStyle:  profile.ForcePathStyle,
			AccessKeyID:     cred.AccessKeyID,
			AccessKeySecret: cred.AccessKeySecret,
		})
		if err != nil {
			return nil, Info{}, err
		}
		return s, Info{
			Profile:        profileName,
			Type:           "s3",
			Endpoint:       profile.Endpoint,
			Region:         profile.Region,
			Bucket:         profile.Bucket,
			Prefix:         prefix,
			ForcePathStyle: profile.ForcePathStyle,
			Credentials:    "global/env",
		}, nil

	default:
		return nil, Info{}, fmt.Errorf("unsupported store profile type %q for profile %q", profile.Type, profileName)
	}
}

func newLegacyStore(root string, project config.Config) (BlobStore, Info, error) {
	switch project.Store.Type {
	case "local":
		path := project.Store.Root
		if path == "" {
			path = ".gamedepot/remote_blobs"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		return NewLocalBlobStore(path), Info{
			Profile: "legacy-local",
			Type:    "local",
			Path:    path,
			Prefix:  project.Store.Prefix,
		}, nil
	default:
		return nil, Info{}, fmt.Errorf("unsupported legacy store.type %q", project.Store.Type)
	}
}

func Check(ctx context.Context, s BlobStore) error {
	const body = "gamedepot store check\n"
	sha := "7f986252ede1b212738fcc23110aff34b02787cc5435c6be3a4f3ed97f0ec1a2"

	if err := s.Put(ctx, sha, strings.NewReader(body)); err != nil {
		return fmt.Errorf("put probe blob: %w", err)
	}

	ok, err := s.Has(ctx, sha)
	if err != nil {
		return fmt.Errorf("head probe blob: %w", err)
	}
	if !ok {
		return fmt.Errorf("probe blob was uploaded but cannot be found")
	}

	r, err := s.Get(ctx, sha)
	if err != nil {
		return fmt.Errorf("get probe blob: %w", err)
	}

	data, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil {
		return fmt.Errorf("read probe blob: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close probe blob: %w", closeErr)
	}
	if string(data) != body {
		return fmt.Errorf("probe blob content mismatch")
	}

	if err := s.Delete(ctx, sha); err != nil {
		return fmt.Errorf("delete probe blob: %w", err)
	}

	return nil
}
