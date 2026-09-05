#!/usr/bin/env bash
set -euo pipefail

echo "========================================================="
echo " AIRO CLOSED-LOOP AUTO-HEALING DEMONSTRATION SCRIPT"
echo "========================================================="

PAYMENT_SVC_URL="http://localhost:8080/api/v1/pay"
CHAOS_URL="http://localhost:8080/chaos/delay"

echo "[1/4] Checking baseline payment service latency..."
for i in {1..5}; do
  curl -s -o /dev/null -w "Request $i Response Time: %{time_total}s\n" "$PAYMENT_SVC_URL"
done

echo ""
echo "[2/4] Injecting 350ms synthetic latency into workload..."
curl -s "$CHAOS_URL?duration_ms=350"
echo ""

echo "[3/4] Generating traffic load under degraded conditions..."
for i in {1..20}; do
  curl -s -o /dev/null "$PAYMENT_SVC_URL" &
  sleep 0.1
done
wait

echo ""
echo "[4/4] Observing AIRO Operator State Machine Transitions..."
echo "Polling RemediationPolicy status phase..."

for attempt in {1..10}; do
  PHASE=$(kubectl get remediationpolicy payment-api-policy -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
  MESSAGE=$(kubectl get remediationpolicy payment-api-policy -o jsonpath='{.status.message}' 2>/dev/null || echo "")
  echo "Current Phase: $PHASE | Message: $MESSAGE"
  
  if [ "$PHASE" == "Healthy" ] && [ $attempt -gt 2 ]; then
    echo "AIRO successfully healed workload and returned to Healthy state!"
    break
  fi
  sleep 3
done

echo ""
echo "Disabling chaos latency..."
curl -s "$CHAOS_URL"
echo ""
echo "Demo complete!"
