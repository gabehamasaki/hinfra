package tailnet

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// unroutableAddr fica na faixa TEST-NET-1 da RFC 5737, reservada para
// documentação e garantidamente não roteável. Derrubar um listener local e
// assumir que a porta passa a recusar conexão não é confiável: em WSL e atrás
// de proxies de loopback o dial ainda é aceito.
const unroutableAddr = "192.0.2.1:9"

func TestRequireRejeitaEnderecoInalcancavel(t *testing.T) {
	err := Require(context.Background(), unroutableAddr, 200*time.Millisecond)
	if err == nil {
		t.Fatal("esperava erro para endereço não roteável")
	}
	if !strings.Contains(err.Error(), ErrNotOnTailnet) {
		t.Fatalf("erro = %v, esperava conter %q", err, ErrNotOnTailnet)
	}
}

func TestRequireAceitaEnderecoQueResponde(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	if err := Require(context.Background(), listener.Addr().String(), time.Second); err != nil {
		t.Fatalf("esperava sucesso contra listener ativo, veio: %v", err)
	}
}

func TestRequireRespeitaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Timeout longo de propósito: quem tem de encerrar o dial é o contexto.
	start := time.Now()
	if err := Require(ctx, unroutableAddr, 30*time.Second); err == nil {
		t.Fatal("esperava erro com contexto já cancelado")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("dial levou %v; contexto cancelado deveria abortar de imediato", elapsed)
	}
}
