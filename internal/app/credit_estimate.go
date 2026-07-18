package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type creditEstimatePayload struct {
	RequiredCredits    int      `json:"required_credits"`
	AvailableCredits   int      `json:"available_credits"`
	MissingCredits     int      `json:"missing_credits"`
	Enough             bool     `json:"enough"`
	RecommendedPackage *Package `json:"recommended_package"`
}

func (a *App) buildCreditEstimate(userID uint, requiredCredits int) (creditEstimatePayload, error) {
	if requiredCredits < 0 {
		requiredCredits = 0
	}
	balance, err := a.lookupBalance(userID)
	if err != nil {
		return creditEstimatePayload{}, err
	}
	missingCredits := maxInt(requiredCredits-balance.AvailableCredits, 0)
	recommendedPackage, err := a.recommendedPackageForMissingCredits(missingCredits)
	if err != nil {
		return creditEstimatePayload{}, err
	}
	return creditEstimatePayload{
		RequiredCredits:    requiredCredits,
		AvailableCredits:   balance.AvailableCredits,
		MissingCredits:     missingCredits,
		Enough:             missingCredits == 0,
		RecommendedPackage: recommendedPackage,
	}, nil
}

func (a *App) recommendedPackageForMissingCredits(missingCredits int) (*Package, error) {
	if missingCredits <= 0 {
		return nil, nil
	}
	if err := a.ensurePackagePresentationColumns(); err != nil {
		return nil, err
	}
	var packages []Package
	if err := a.db.
		Where("is_active = ? AND credits > ?", true, 0).
		Order("credits asc, sort_order asc, id asc").
		Find(&packages).Error; err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, nil
	}
	bestIndex := len(packages) - 1
	for index := range packages {
		if packages[index].Credits >= missingCredits {
			bestIndex = index
			break
		}
	}
	recommended := packages[bestIndex]
	return &recommended, nil
}

func writeCreditsInsufficientError(c *gin.Context, estimate creditEstimatePayload) {
	c.Set(requestLogErrorCodeKey, "credits_insufficient")
	c.Set(requestLogErrorMessageKey, "点数不足，请先充值")
	payload := gin.H{
		"error": gin.H{
			"code":                "credits_insufficient",
			"message":             "点数不足，请先充值",
			"required_credits":    estimate.RequiredCredits,
			"available_credits":   estimate.AvailableCredits,
			"missing_credits":     estimate.MissingCredits,
			"enough":              estimate.Enough,
			"recommended_package": estimate.RecommendedPackage,
		},
		"required_credits":    estimate.RequiredCredits,
		"available_credits":   estimate.AvailableCredits,
		"missing_credits":     estimate.MissingCredits,
		"enough":              estimate.Enough,
		"recommended_package": estimate.RecommendedPackage,
	}
	c.JSON(http.StatusConflict, payload)
}
