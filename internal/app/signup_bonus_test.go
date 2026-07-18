package app

import (
	"testing"

	"gorm.io/gorm"
)

func assertSignupBonusTransaction(t *testing.T, db *gorm.DB, userID uint) CreditTransaction {
	t.Helper()
	var transactions []CreditTransaction
	if err := db.Where("user_id = ? AND type = ?", userID, CreditTransactionTypeSignupBonus).Find(&transactions).Error; err != nil {
		t.Fatalf("load signup bonus transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("expected one signup bonus transaction, got %d", len(transactions))
	}
	transaction := transactions[0]
	if transaction.Amount != signupBonusCredits ||
		transaction.BalanceAfter != signupBonusCredits ||
		transaction.Reason != signupBonusReason ||
		transaction.RelatedType != "user" ||
		transaction.RelatedID != userID {
		t.Fatalf("unexpected signup bonus transaction: %+v", transaction)
	}
	return transaction
}

func countSignupBonusTransactions(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&CreditTransaction{}).
		Where("user_id = ? AND type = ?", userID, CreditTransactionTypeSignupBonus).
		Count(&count).Error; err != nil {
		t.Fatalf("count signup bonus transactions: %v", err)
	}
	return count
}
