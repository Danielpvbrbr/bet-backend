package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

// --- DTOs ---
type WagerRequest struct {
	ProviderID          string `json:"providerId"`
	ExternalTransaction string `json:"externalTransactionId"`
	PlayerID            string `json:"playerId"`
	WalletID            string `json:"walletId"`
	Kind                string `json:"kind"`
	Money               struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"money"`
}

type CreateWalletRequest struct {
	PlayerID string `json:"playerId"`
	Currency string `json:"currency"`
}

type WalletResponse struct {
	ID        string `json:"id"`
	PlayerID  string `json:"playerId"`
	Currency  string `json:"currency"`
	Balance   string `json:"balance"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type LedgerEntryResponse struct {
	ID            string `json:"id"`
	TransactionID string `json:"transactionId"`
	EntryType     string `json:"entryType"`
	Amount        string `json:"amount"`
	BalanceAfter  string `json:"balanceAfter"`
	CreatedAt     string `json:"createdAt"`
}

type StatementResponse struct {
	WalletID string                `json:"walletId"`
	Entries  []LedgerEntryResponse `json:"entries"`
}

// --- HANDLER & LÓGICA ---
type AppHandler struct {
	db *pgxpool.Pool
}

func NewAppHandler(db *pgxpool.Pool) *AppHandler {
	return &AppHandler{db: db}
}

func (h *AppHandler) HandleBet(w http.ResponseWriter, r *http.Request) {
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		http.Error(w, `{"error": "Idempotency-Key header is required"}`, http.StatusBadRequest)
		return
	}

	var req WagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "json_invalido"}`, http.StatusBadRequest)
		return
	}

	money, err := ParseExternalMoney(req.Money.Amount, req.Money.Currency)
	if err != nil {
		http.Error(w, `{"error": "valor_invalido_ou_negativo"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		http.Error(w, `{"error": "database_connection_failed"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	var balanceCents int64
	var version int
	err = tx.QueryRow(ctx, "SELECT balance, version FROM wallets WHERE id = $1", req.WalletID).Scan(&balanceCents, &version)
	if err != nil {
		http.Error(w, `{"error": "wallet_not_found"}`, http.StatusNotFound)
		return
	}

	wallet := RestoreWallet(req.WalletID, req.PlayerID, NewMoneyFromCents(balanceCents, req.Money.Currency), version, time.Now())

	var ledgerType string
	switch req.Kind {
	case "BET":
		if err := wallet.Debit(money); err != nil {
			http.Error(w, `{"error": "insufficient_funds"}`, http.StatusUnprocessableEntity)
			return
		}
		ledgerType = "DEBIT"
	case "WIN", "REFUND":
		if err := wallet.Credit(money); err != nil {
			http.Error(w, `{"error": "currency_mismatch"}`, http.StatusUnprocessableEntity)
			return
		}
		ledgerType = "CREDIT"
	default:
		http.Error(w, `{"error": "tipo_de_transacao_invalido"}`, http.StatusBadRequest)
		return
	}

	// 1. Gravar transação
	transactionID := uuid.New().String()
	_, err = tx.Exec(ctx, `
		INSERT INTO wager_transactions 
		(id, provider_id, external_id, idempotency_key, wallet_id, player_id, kind, amount, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, transactionID, req.ProviderID, req.ExternalTransaction, idemKey, wallet.id, req.PlayerID, req.Kind, money.Cents(), money.Currency(), "PROCESSED")

	if err != nil {
		http.Error(w, `{"error": "transacao_duplicada_idempotencia_rejeitada"}`, http.StatusConflict)
		return
	}

	// 2. Atualizar saldo com Lock Otimista
	cmd, err := tx.Exec(ctx, "UPDATE wallets SET balance = $1, version = $2 WHERE id = $3 AND version = $4",
		wallet.balance.Cents(), wallet.version, wallet.id, version)

	if err != nil || cmd.RowsAffected() == 0 {
		http.Error(w, `{"error": "concurrent_conflict"}`, http.StatusConflict)
		return
	}

	// 3. Gravar entrada no Ledger Financeiro
	_, err = tx.Exec(ctx, `
		INSERT INTO wallet_ledger_entries (id, wallet_id, transaction_id, entry_type, amount, balance_after)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.New().String(), wallet.id, transactionID, ledgerType, money.Cents(), wallet.balance.Cents())

	if err != nil {
		fmt.Println("Erro real do ledger:", err)
		http.Error(w, `{"error": "falha_ao_salvar_ledger"}`, http.StatusInternalServerError)
		return
	}

	// 4. Salvar evento no Outbox
	eventPayload := fmt.Sprintf(`{"transactionId": "%s", "walletId": "%s", "balance": %d, "kind": "%s"}`,
		transactionID, wallet.id, wallet.balance.Cents(), req.Kind)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (id, aggregate_type, aggregate_id, payload)
		VALUES ($1, $2, $3, $4)
	`, uuid.New().String(), "WagerTransaction", transactionID, eventPayload)

	if err != nil {
		fmt.Println("Erro real do outbox:", err)
		http.Error(w, `{"error": "falha_ao_salvar_outbox"}`, http.StatusInternalServerError)
		return
	}

	// 5. Commit atômico
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, `{"error": "falha_ao_confirmar_transacao"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"transactionId": "%s", "status": "PROCESSED", "balance": {"amount": "%.2f", "currency": "%s"}}`,
		transactionID, float64(wallet.balance.Cents())/100, wallet.balance.Currency())
}

func (h *AppHandler) HandleCreateWallet(w http.ResponseWriter, r *http.Request) {
	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "json_invalido"}`, http.StatusBadRequest)
		return
	}

	if req.PlayerID == "" || req.Currency == "" {
		http.Error(w, `{"error": "playerId e currency sao obrigatorios"}`, http.StatusBadRequest)
		return
	}

	walletID := uuid.New().String()

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO wallets (id, player_id, currency, balance, version, created_at, updated_at)
		VALUES ($1, $2, $3, 0, 1, NOW(), NOW())
	`, walletID, req.PlayerID, req.Currency)

	if err != nil {
		http.Error(w, `{"error": "carteira_ja_existe_para_este_jogador_e_moeda"}`, http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"walletId": "%s", "playerId": "%s", "currency": "%s", "balance": "0.00"}`,
		walletID, req.PlayerID, req.Currency)
}

func (h *AppHandler) HandleGetWallet(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("id")
	if walletID == "" {
		http.Error(w, `{"error": "wallet_id_obrigatorio"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var (
		id        string
		playerID  string
		currency  string
		balance   int64
		version   int
		updatedAt time.Time
	)

	err := h.db.QueryRow(ctx, `
		SELECT id, player_id, currency, balance, version, updated_at 
		FROM wallets 
		WHERE id = $1
	`, walletID).Scan(&id, &playerID, &currency, &balance, &version, &updatedAt)

	if err != nil {
		http.Error(w, `{"error": "wallet_not_found"}`, http.StatusNotFound)
		return
	}

	res := WalletResponse{
		ID:        id,
		PlayerID:  playerID,
		Currency:  currency,
		Balance:   fmt.Sprintf("%.2f", float64(balance)/100),
		Version:   version,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (h *AppHandler) HandleGetStatement(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("id")
	if walletID == "" {
		http.Error(w, `{"error": "wallet_id_obrigatorio"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	rows, err := h.db.Query(ctx, `
		SELECT id, transaction_id, entry_type, amount, balance_after, created_at 
		FROM wallet_ledger_entries 
		WHERE wallet_id = $1 
		ORDER BY created_at DESC
	`, walletID)
	if err != nil {
		http.Error(w, `{"error": "falha_ao_buscar_extrato"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := make([]LedgerEntryResponse, 0)
	for rows.Next() {
		var (
			id            string
			transactionID string
			entryType     string
			amount        int64
			balanceAfter  int64
			createdAt     time.Time
		)

		if err := rows.Scan(&id, &transactionID, &entryType, &amount, &balanceAfter, &createdAt); err != nil {
			http.Error(w, `{"error": "falha_ao_processar_extrato"}`, http.StatusInternalServerError)
			return
		}

		entries = append(entries, LedgerEntryResponse{
			ID:            id,
			TransactionID: transactionID,
			EntryType:     entryType,
			Amount:        fmt.Sprintf("%.2f", float64(amount)/100),
			BalanceAfter:  fmt.Sprintf("%.2f", float64(balanceAfter)/100),
			CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(StatementResponse{
		WalletID: walletID,
		Entries:  entries,
	})
}

// --- MIDDLEWARE DE AUTENTICAÇÃO ---
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error": "unauthorized_missing_token"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			http.Error(w, `{"error": "unauthorized_invalid_token"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func NewDB(lc fx.Lifecycle) *pgxpool.Pool {
	conn := os.Getenv("DATABASE_URL")
	if conn == "" {
		conn = "postgres://postgres:postgres@127.0.0.1:5433/betdb?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), conn)
	if err != nil {
		log.Fatalf("Falha crítica ao conectar no PostgreSQL: %v", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			pool.Close()
			return nil
		},
	})

	return pool
}

func StartOutboxWorker(db *pgxpool.Pool) {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			ctx := context.Background()

			var id, payload string
			err := db.QueryRow(ctx, "SELECT id, payload::text FROM outbox WHERE processed_at IS NULL LIMIT 1 FOR UPDATE SKIP LOCKED").Scan(&id, &payload)

			if err == nil {
				fmt.Printf("\n [WORKER SQS] Enviando evento de saldo atualizado para a fila: %s\n", payload)
				db.Exec(ctx, "UPDATE outbox SET processed_at = NOW() WHERE id = $1", id)
			}
		}
	}()
}

func StartServer(lc fx.Lifecycle, h *AppHandler) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /wagering/transactions", h.HandleBet)
	mux.HandleFunc("POST /wallets", h.HandleCreateWallet)
	mux.HandleFunc("GET /wallets/{id}", h.HandleGetWallet)
	mux.HandleFunc("GET /wallets/{id}/statement", h.HandleGetStatement)

	server := &http.Server{
		Addr:    ":8080",
		Handler: AuthMiddleware(mux),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			StartOutboxWorker(h.db)
			go server.ListenAndServe()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
}

func main() {
	fx.New(
		fx.Provide(NewDB, NewAppHandler),
		fx.Invoke(StartServer),
	).Run()
}
