package app

import "gorm.io/gorm"

const (
	signupBonusCredits = 5
	signupBonusReason  = "新用户注册体验点数"
)

func createSignupBonusTx(tx *gorm.DB, userID uint) error {
	balance := CreditBalance{UserID: userID, AvailableCredits: signupBonusCredits}
	if err := tx.Create(&balance).Error; err != nil {
		return err
	}
	transaction := CreditTransaction{
		UserID:       userID,
		Type:         CreditTransactionTypeSignupBonus,
		Amount:       signupBonusCredits,
		BalanceAfter: signupBonusCredits,
		Reason:       signupBonusReason,
		RelatedType:  "user",
		RelatedID:    userID,
	}
	return tx.Create(&transaction).Error
}
