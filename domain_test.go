package main

import (
	"testing"
	"time"
)

func TestWallet_Debit_InsufficientFunds(t *testing.T) {
	initialMoney := NewMoneyFromCents(5000, "BRL") // R$ 50,00
	wallet := RestoreWallet("wallet-1", "player-1", initialMoney, 1, time.Now())

	debitMoney := NewMoneyFromCents(6000, "BRL") // R$ 60,00
	err := wallet.Debit(debitMoney)

	if err != ErrInsufficientFunds {
		t.Fatalf("esperado erro ErrInsufficientFunds, mas obteve: %v", err)
	}

	if wallet.balance.Cents() != 5000 {
		t.Fatalf("saldo nao deveria mudar, esperado 5000 mas obteve: %d", wallet.balance.Cents())
	}
}

func TestWallet_Credit_Success(t *testing.T) {
	initialMoney := NewMoneyFromCents(1000, "BRL") // R$ 10,00
	wallet := RestoreWallet("wallet-1", "player-1", initialMoney, 1, time.Now())

	creditMoney := NewMoneyFromCents(2500, "BRL") // R$ 25,00
	err := wallet.Credit(creditMoney)

	if err != nil {
		t.Fatalf("erro inesperado ao creditar: %v", err)
	}

	if wallet.balance.Cents() != 3500 {
		t.Fatalf("esperado saldo 3500, mas obteve: %d", wallet.balance.Cents())
	}

	if wallet.version != 2 {
		t.Fatalf("versao deveria ter incrementado para 2, mas esta: %d", wallet.version)
	}
}

func TestParseExternalMoney_Precision(t *testing.T) {
	money, err := ParseExternalMoney("19.99", "BRL")
	if err != nil {
		t.Fatalf("erro ao converter valor: %v", err)
	}

	if money.Cents() != 1999 {
		t.Fatalf("esperado 1999 centavos, obteve: %d", money.Cents())
	}
}
