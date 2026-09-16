---
name: payment-integration
description: >-
  Integrate payments for Vietnam and global markets: SePay/VietQR flows, Stripe checkout
  and webhooks, Polar for digital products — with webhook verification, idempotency, and
  reconciliation. Use when adding or fixing any payment flow. Keywords: payment, Stripe,
  SePay, VietQR, webhook, Polar, test cards. Dùng khi tích hợp thanh toán, SePay, VietQR,
  Stripe, webhook thanh toán, đối soát giao dịch.
license: MIT
version: 1
---

# Payment Integration

Wire up payment flows correctly: Vietnam bank transfer (SePay/VietQR), Stripe checkout for cards, and Polar for digital products — with verified webhooks, idempotent handlers, and reconciliation.

## When to use
- Adding or changing a checkout, top-up, or subscription flow
- Receiving webhooks from Stripe, SePay, or another gateway
- Reconciling payments against orders, or handling duplicate/late events
- Choosing between gateways for Vietnam, global cards, or digital goods

## When NOT to use
- General backend reliability around the flow → `backend-development`
- Shopify in-app payments and billing surfaces → `shopify`
- Ledger/schema design deep dive → `databases`

## Workflow
1. Choose the rail: Vietnam domestic bank transfer → SePay with VietQR (customers scan or pay by account number; the transfer content is your matching key); global cards/wallets → Stripe Checkout (hosted, lowest PCI burden); digital products/memberships → Polar (merchant of record handles tax).
2. Model payments as a state machine: `pending → paid → fulfilled`, with `expired` and `failed`; persist every state change; never derive state from a single webhook.
3. Create the intent server-side: amount and currency are computed by the server from the order; the client only references an order ID and never sends amounts.
4. Secure the webhook: verify the signature/HMAC before running business logic; acknowledge fast (2xx) and process asynchronously when heavy; store the raw event for replay.
5. Make handlers idempotent: unique constraint on the provider event ID; on duplicates, acknowledge without re-fulfilling; apply state transitions forward only, never backward.
6. Match Vietnam transfers: SePay webhooks carry the bank transfer content — match by a unique per-order reference code embedded in that content, not by amount alone; handle partial and over payments explicitly.
7. Reconcile daily: pull the provider's transaction list, diff against local paid records, alert on mismatches, and keep a manual-review queue for unmatched credits.
8. Test with provider sandboxes and test cards (Stripe's 4242-style test numbers plus decline and 3DS variants); simulate duplicate, late, and out-of-order webhooks before shipping.
9. Document the flow: sequence diagram, state machine, retry/replay runbook, and who gets notified when reconciliation drifts.

## Output
- A payment integration with: state machine, verified webhook handlers, idempotency keys, matching rules, a reconciliation job, and a test matrix covering duplicates and declines.

## Routing
- Gateway-side backend design (queues, retries) → `backend-development`
- Schema for ledgers and payment tables → `databases`
- Security review (signature checks, secrets) → `security-audit`

## Guardrails
- Never log full card data, tokens, or webhook secrets.
- Amount, currency, and payee are server-computed constants; client input can only reference an order.
- Fulfillment happens once: enforce idempotency at the storage layer, not by application checks alone.
- Keep provider API keys per environment; a test key in production config is a release blocker.
