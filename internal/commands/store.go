package commands

import (
	"context"
	"fmt"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/store"
)

func StoreInfo(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	printStoreInfo(a.StoreInfo)
	return nil
}

func StoreCheck(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	printStoreInfo(a.StoreInfo)
	if err := store.Check(ctx, a.Store); err != nil {
		return err
	}

	fmt.Println("status: ok")
	return nil
}

func printStoreInfo(info store.Info) {
	fmt.Printf("profile: %s\n", info.Profile)
	fmt.Printf("type: %s\n", info.Type)
	switch info.Type {
	case "local":
		fmt.Printf("path: %s\n", info.Path)
	case "s3":
		fmt.Printf("endpoint: %s\n", info.Endpoint)
		fmt.Printf("region: %s\n", info.Region)
		fmt.Printf("bucket: %s\n", info.Bucket)
		fmt.Printf("prefix: %s\n", info.Prefix)
		fmt.Printf("force_path_style: %t\n", info.ForcePathStyle)
	}
}
