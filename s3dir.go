package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jessevdk/go-flags"
	"github.com/relvacode/s3dir/pkg/s3dir"
	"net/http"
	"os"
)

type CLI struct {
	ListenAddress string `long:"listen-address" env:"LISTEN_ADDRESS" default:"127.0.0.1:9001" description:"Listen on this address"`
	Endpoint      string `long:"endpoint" env:"ENDPOINT" description:"AWS S3 endpoint"`
	Application   string `long:"application" env:"APPLICATION" default:"S3" description:"Your application name. Used to configure the page title."`
}

func Main() error {
	var cli CLI

	p := flags.NewParser(&cli, flags.HelpFlag)
	_, err := p.Parse()
	if err != nil {
		return err
	}

	var configOpts []func(*config.LoadOptions) error

	if cli.Endpoint != "" {
		configOpts = append(configOpts, config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if service != s3.ServiceID {
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}

			return aws.Endpoint{
				URL:           cli.Endpoint,
				SigningRegion: "us-west-2",
			}, nil
		})))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), configOpts...)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:    cli.ListenAddress,
		Handler: s3dir.New(s3.NewFromConfig(cfg), &s3dir.Renderer{Title: cli.Application}),
	}

	return httpServer.ListenAndServe()
}

func main() {
	err := Main()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
