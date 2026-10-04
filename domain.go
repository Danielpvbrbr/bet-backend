package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidAmount     = errors.New("formato de valor invalido")
	ErrNegativeAmount    = errors.New("valores negativos nao permitidos")
	ErrCurrencyMismatch  = errors.New("moedas incompativeis")
	ErrInsufficientFunds = errors.New("saldo insuficiente")
	ErrConcurrentUpdate  = errors.New("conflito de concorrencia")
)

// --- MONEY ---
type Money struct {
	cents    int64
	currency string
}

func ParseExternalMoney(amount, currency string) (Money, error) {
	parts := strings.Split(amount, ".")
	if len(parts) != 2 || len(parts[1]) != 2 {
		return Money{}, ErrInvalidAmount
	}
	cents, err := strconv.ParseInt(parts[0]+parts[1], 10, 64)
	if err != nil || cents < 0 {
		return Money{}, ErrNegativeAmount
	}
	return Money{cents: cents, currency: currency}, nil
}

func NewMoneyFromCents(cents int64, currency string) Money {
	return Money{cents: cents, currency: currency}
}

func (m Money) Cents() int64      { return m.cents }
func (m Money) Currency() string  { return m.currency }
func (m Money) Add(o Money) Money { return Money{cents: m.cents + o.cents, currency: m.currency} }
func (m Money) Sub(o Money) Money { return Money{cents: m.cents - o.cents, currency: m.currency} }

// --- WALLET ---
type Wallet struct {
	id        string
	playerId  string
	balance   Money
	version   int
	updatedAt time.Time
}

func RestoreWallet(id, playerId string, balance Money, version int, updatedAt time.Time) *Wallet {
	return &Wallet{id: id, playerId: playerId, balance: balance, version: version, updatedAt: updatedAt}
}

func (w *Wallet) Debit(amount Money) error {
	if amount.currency != w.balance.currency {
		return ErrCurrencyMismatch
	}
	newBalance := w.balance.Sub(amount)
	if newBalance.cents < 0 {
		return ErrInsufficientFunds
	}
	w.balance = newBalance
	w.version++
	w.updatedAt = time.Now().UTC()
	return nil
}

// --- TRANSACTION ---
type WagerTransaction struct {
	id             string
	providerID     string
	externalID     string
	idempotencyKey string
	walletID       string
	kind           string
	money          Money
	status         string
}

func (w *Wallet) Credit(amount Money) error {
	if amount.currency != w.balance.currency {
		return ErrCurrencyMismatch
	}
	w.balance = w.balance.Add(amount)
	w.version++
	w.updatedAt = time.Now().UTC()
	return nil
}

func NewWagerTransaction(id, provID, extID, idemKey, wallID, kind string, m Money) *WagerTransaction {
	return &WagerTransaction{
		id: id, providerID: provID, externalID: extID, idempotencyKey: idemKey,
		walletID: wallID, kind: kind, money: m, status: "PENDING",
	}
}
