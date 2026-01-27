package main

import (
	cfg "github.com/conductorone/baton-bitbucket/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/config"
)

func main() {
	config.Generate("bitbucket", cfg.Config)
}
