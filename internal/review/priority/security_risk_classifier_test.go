// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority_test

import (
	"testing"

	"github.com/scandrix/backend/internal/review/priority"
	"github.com/stretchr/testify/assert"
)

func TestSecurityRiskClassifier_Classification(t *testing.T) {
	classifier := priority.NewSecurityRiskClassifier()

	t.Run("Auth File Classified as Critical", func(t *testing.T) {
		res := classifier.ClassifyFile(
			"internal/auth/jwt_verifier.go",
			"package auth\nfunc Verify(token string) bool { return true }",
			"@@ -1,2 +1,3 @@\n+func Verify(token string) bool { return true }\n",
		)

		assert.Equal(t, priority.DomainAuthCredentials, res.PrimaryDomain)
		assert.Equal(t, priority.TierCritical, res.RecommendedTier)
		assert.GreaterOrEqual(t, res.DomainMultiplier, 4.0)
	})

	t.Run("Database Migration Classified as Critical", func(t *testing.T) {
		res := classifier.ClassifyFile(
			"migrations/001_create_users.sql",
			"CREATE TABLE users (id UUID PRIMARY KEY, email TEXT NOT NULL);",
			"@@ -0,0 +1,3 @@\n+CREATE TABLE users (id UUID PRIMARY KEY);\n",
		)

		assert.Equal(t, priority.DomainDatabaseStorage, res.PrimaryDomain)
		assert.Equal(t, priority.TierCritical, res.RecommendedTier)
		assert.GreaterOrEqual(t, res.DomainMultiplier, 3.0)
	})

	t.Run("Payment and Billing Classified as Critical", func(t *testing.T) {
		res := classifier.ClassifyFile(
			"pkg/payment/stripe_client.go",
			"package payment\nfunc ChargeCard() {}",
			"@@ -1,2 +1,3 @@\n+func ChargeCard() {}\n",
		)

		assert.Equal(t, priority.DomainPaymentBilling, res.PrimaryDomain)
		assert.Equal(t, priority.TierCritical, res.RecommendedTier)
	})

	t.Run("Testing Fixture Classified as Optional", func(t *testing.T) {
		res := classifier.ClassifyFile(
			"pkg/payment/stripe_client_test.go",
			"package payment_test\nfunc TestCharge(t *testing.T) {}",
			"@@ -1,2 +1,3 @@\n+func TestCharge(t *testing.T) {}\n",
		)

		assert.Equal(t, priority.DomainTestingFixture, res.PrimaryDomain)
		assert.Equal(t, priority.TierOptional, res.RecommendedTier)
		assert.LessOrEqual(t, res.DomainMultiplier, 0.5)
	})

	t.Run("Documentation Classified as Optional", func(t *testing.T) {
		res := classifier.ClassifyFile(
			"docs/architecture/adr-001.md",
			"# Architecture Decisions",
			"@@ -1,2 +1,3 @@\n+# Architecture Decisions\n",
		)

		assert.Equal(t, priority.DomainDocumentation, res.PrimaryDomain)
		assert.Equal(t, priority.TierOptional, res.RecommendedTier)
	})

	t.Run("High Cyclomatic Branching Complexity Multiplier", func(t *testing.T) {
		patchWithManyBranches := `@@ -1,50 +1,50 @@
+ if a {
+     if b && c {
+         for i := 0; i < 10; i++ {
+             switch x {
+             case 1:
+             case 2:
+             case 3:
+             case 4:
+             case 5:
+             }
+         }
+     } else if d || e {
+         select {
+         case <-ch1:
+         case <-ch2:
+         }
+     }
+ }
`
		res := classifier.ClassifyFile(
			"pkg/network/gateway.go",
			"package network",
			patchWithManyBranches,
		)

		assert.GreaterOrEqual(t, res.ComplexityMultiplier, 1.5)
		assert.Greater(t, res.CompositeRiskScore, res.DomainMultiplier)
	})
}
