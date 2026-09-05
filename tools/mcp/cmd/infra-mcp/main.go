package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gabehamasaki/infra/tools/mcp/internal/config"
	"github.com/gabehamasaki/infra/tools/mcp/internal/server"
	"github.com/gabehamasaki/infra/tools/mcp/internal/tailnet"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	selftest := flag.Bool("selftest", false, "valida config, kubeconfig e dial na tailnet")
	smoke := flag.Bool("smoke", false, "executa ferramentas de leitura no cwd atual e imprime JSON")
	flag.Parse()

	runtime, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if *selftest {
		if err := runSelfTest(runtime); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Println("selftest ok")
		return
	}

	if err := runtime.ValidateFiles(); err != nil {
		log.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	env := &server.Env{Runtime: runtime, CWD: cwd}

	if *smoke {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := server.RunSmoke(ctx, env); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "infra", Version: "0.1.0"}, nil)
	server.Register(srv, env)

	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func runSelfTest(runtime *config.Runtime) error {
	if err := runtime.ValidateFiles(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tailnet.Require(ctx, runtime.TailnetAPI, 2*time.Second); err != nil {
		return err
	}
	return nil
}
