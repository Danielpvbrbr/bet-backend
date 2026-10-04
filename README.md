# Betting Core Engine API

Sistema de transações financeiras e controle de saldo para apostas esportivas, construído com foco em consistência atômica, isolamento e integridade monetária.

## Decisões de Arquitetura e Engenharia

1. **Prevenção de Floating-Point Inaccuracies**:
   - Tratamento de moeda baseado em inteiros (centavos em `int64`), eliminando riscos de arredondamento comuns em tipos `float`.
2. **Controle de Concorrência Otimista (OCC)**:
   - Uso de coluna `version` na tabela `wallets`. Tentativas simultâneas de débito que colidam com versões defasadas retornam conflito imediato (`HTTP 409 Conflict`), prevenindo *double-spending*.
3. **Idempotência Rigorosa**:
   - Uso do cabeçalho `Idempotency-Key` atrelado a uma constraint `UNIQUE` no banco de dados. Requisições repetidas de provedores são bloqueadas sem cobrança duplicada.
4. **Ledger Financeiro Imutável**:
   - Registro detalhado de débitos (`DEBIT`) e créditos (`CREDIT`) em partidas simples/dobradas com saldo auditável após cada transação (`balance_after`).
5. **Transactional Outbox Pattern**:
   - Persistência atômica do evento na tabela `outbox` na mesma transação SQL da aposta, com worker assíncrono para publicação distribuída (simulação SQS).

## Como Executar

### 1. Subir Infraestrutura e Migrações
```bash
docker compose up -d