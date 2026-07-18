package ecommerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrSKUVersionConflict = errors.New("sku_version_conflict")
	ErrDefaultSKUDisable  = errors.New("default_sku_disable_forbidden")
)

type SKUValueInput struct {
	ID        uint   `json:"id,omitempty"`
	Name      string `json:"name"`
	Code      string `json:"code,omitempty"`
	Status    string `json:"status,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}
type SKUDimensionInput struct {
	ID        uint            `json:"id,omitempty"`
	Name      string          `json:"name"`
	Status    string          `json:"status,omitempty"`
	SortOrder int             `json:"sort_order,omitempty"`
	Values    []SKUValueInput `json:"values"`
}
type SKUMatrixInput struct {
	ExpectedVersion int                 `json:"expected_version"`
	Dimensions      []SKUDimensionInput `json:"dimensions"`
}
type SKUMatrixCandidate struct {
	Key            string             `json:"key"`
	Code           string             `json:"code"`
	SKUID          uint               `json:"sku_id,omitempty"`
	Status         string             `json:"status,omitempty"`
	Specifications []SKUSpecification `json:"specifications"`
	SKU            *CommerceSKU       `json:"-"`
}
type SKUMatrixPreview struct {
	Version   int                  `json:"version"`
	Add       []SKUMatrixCandidate `json:"add"`
	Keep      []SKUMatrixCandidate `json:"keep"`
	Conflicts []SKUMatrixCandidate `json:"conflicts"`
	Disable   []SKUMatrixCandidate `json:"disable"`
}
type SKUConfig struct {
	Version      int                    `json:"version"`
	DefaultSKUID uint                   `json:"default_sku_id,omitempty"`
	DefaultKnown bool                   `json:"default_known,omitempty"`
	Dimensions   []CommerceSKUDimension `json:"dimensions"`
	Values       []CommerceSKUValue     `json:"values"`
	SKUs         []CommerceSKU          `json:"skus"`
}

func (s *Service) ownedProduct(ctx context.Context, userID, productID uint, db *gorm.DB) (CommerceProduct, error) {
	var p CommerceProduct
	if err := db.WithContext(ctx).First(&p, productID).Error; err != nil {
		return p, mapNotFound(err)
	}
	if p.UserID != userID {
		return p, ErrOwnershipMismatch
	}
	return p, nil
}

func normalizeMatrix(input SKUMatrixInput) (SKUMatrixInput, error) {
	seen := map[string]bool{}
	total := 1
	active := make([]SKUDimensionInput, 0, len(input.Dimensions))
	for di, d := range input.Dimensions {
		d.Name = strings.TrimSpace(d.Name)
		if d.Status == "disabled" {
			continue
		}
		if d.Name == "" || seen[d.Name] {
			return input, invalidField("dimensions", "规格维度名称不能为空或重复")
		}
		seen[d.Name] = true
		vs := make([]SKUValueInput, 0, len(d.Values))
		valueSeen := map[string]bool{}
		for vi, v := range d.Values {
			if v.Status == "disabled" {
				continue
			}
			v.Name = strings.TrimSpace(v.Name)
			v.Code = strings.TrimSpace(v.Code)
			if v.Name == "" || valueSeen[v.Name] {
				return input, invalidField("values", "规格值不能为空或重复")
			}
			valueSeen[v.Name] = true
			v.SortOrder = vi
			vs = append(vs, v)
		}
		if len(vs) > 20 {
			return input, invalidField("values", "每个规格维度最多启用 20 个值")
		}
		if len(vs) == 0 {
			return input, invalidField("values", "启用维度至少需要一个启用值")
		}
		d.Values = vs
		d.SortOrder = di
		d.Status = "active"
		active = append(active, d)
		total *= len(vs)
	}
	if len(active) > 3 {
		return input, invalidField("dimensions", "最多启用 3 个规格维度")
	}
	if len(active) == 0 {
		total = 0
	}
	if total > 100 {
		return input, invalidField("dimensions", "有效 SKU 组合最多 100 个")
	}
	input.Dimensions = active
	return input, nil
}

func candidates(input SKUMatrixInput) []SKUMatrixCandidate {
	result := []SKUMatrixCandidate{{}}
	for _, d := range input.Dimensions {
		next := make([]SKUMatrixCandidate, 0, len(result)*len(d.Values))
		for _, base := range result {
			for _, v := range d.Values {
				c := base
				c.Specifications = append(append([]SKUSpecification{}, base.Specifications...), SKUSpecification{Dimension: d.Name, Value: v.Name})
				if c.Code == "" && v.Code != "" {
					c.Code = v.Code
				}
				next = append(next, c)
			}
		}
		result = next
	}
	for i := range result {
		parts := make([]string, len(result[i].Specifications))
		for j, s := range result[i].Specifications {
			parts[j] = s.Dimension + "=" + s.Value
		}
		result[i].Key = strings.Join(parts, "|")
	}
	return result
}

func digestMatrix(input SKUMatrixInput) string {
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) existingCandidates(ctx context.Context, db *gorm.DB, userID, productID uint) ([]SKUMatrixCandidate, []CommerceSKU, error) {
	var skus []CommerceSKU
	if err := db.WithContext(ctx).Where("user_id=? AND product_id=?", userID, productID).Order("id").Find(&skus).Error; err != nil {
		return nil, nil, err
	}
	result := make([]SKUMatrixCandidate, 0, len(skus))
	for i := range skus {
		var rows []struct {
			DimensionID, ValueID uint
			Dimension, Value     string
		}
		err := db.WithContext(ctx).Table("commerce_sku_value_links l").Select("d.id dimension_id,v.id value_id,d.name dimension,v.name value").Joins("JOIN commerce_sku_values v ON v.id=l.value_id").Joins("JOIN commerce_sku_dimensions d ON d.id=v.dimension_id").Where("l.sku_id=?", skus[i].ID).Order("d.sort_order,v.sort_order").Scan(&rows).Error
		if err != nil {
			return nil, nil, err
		}
		specs := make([]SKUSpecification, len(rows))
		parts := make([]string, len(rows))
		for j, r := range rows {
			specs[j] = SKUSpecification{r.DimensionID, r.ValueID, r.Dimension, r.Value}
			parts[j] = r.Dimension + "=" + r.Value
		}
		skus[i].Specification = specs
		if err := db.WithContext(ctx).Table("commerce_projects p").Select("COUNT(a.id)").Joins("JOIN commerce_assets a ON a.project_id=p.id AND a.sku_id=? AND a.deleted_at IS NULL", skus[i].ID).Where("p.user_id=? AND p.product_id=? AND p.deleted_at IS NULL", userID, productID).Scan(&skus[i].AssetCount).Error; err != nil {
			return nil, nil, err
		}
		var defaults int64
		if err := db.WithContext(ctx).Model(&CommerceProject{}).Where("user_id=? AND product_id=? AND default_sku_id=?", userID, productID, skus[i].ID).Count(&defaults).Error; err != nil {
			return nil, nil, err
		}
		skus[i].IsDefault = defaults > 0
		result = append(result, SKUMatrixCandidate{Key: strings.Join(parts, "|"), Code: skus[i].Code, SKUID: skus[i].ID, Status: skus[i].Status, Specifications: specs, SKU: &skus[i]})
	}
	return result, skus, nil
}

func (s *Service) previewWithDB(ctx context.Context, db *gorm.DB, userID, productID uint, input SKUMatrixInput) (SKUMatrixPreview, error) {
	p, err := s.ownedProduct(ctx, userID, productID, db)
	if err != nil {
		return SKUMatrixPreview{}, err
	}
	input, err = normalizeMatrix(input)
	if err != nil {
		return SKUMatrixPreview{}, err
	}
	desired := candidates(input)
	existing, _, err := s.existingCandidates(ctx, db, userID, productID)
	if err != nil {
		return SKUMatrixPreview{}, err
	}
	byKey := map[string]SKUMatrixCandidate{}
	existingCodes := map[string]string{}
	for _, e := range existing {
		existingCodes[e.Code] = e.Key
		if e.SKU.Status == "active" {
			byKey[e.Key] = e
		}
	}
	preview := SKUMatrixPreview{Version: p.SKUVersion, Add: []SKUMatrixCandidate{}, Keep: []SKUMatrixCandidate{}, Conflicts: []SKUMatrixCandidate{}, Disable: []SKUMatrixCandidate{}}
	codes := map[string]bool{}
	for _, c := range desired {
		if e, ok := byKey[c.Key]; ok {
			preview.Keep = append(preview.Keep, e)
			delete(byKey, c.Key)
		} else {
			ownerKey, taken := existingCodes[c.Code]
			if c.Code != "" && (codes[c.Code] || (taken && ownerKey != c.Key)) {
				preview.Conflicts = append(preview.Conflicts, c)
			} else {
				preview.Add = append(preview.Add, c)
				codes[c.Code] = true
			}
		}
	}
	for _, e := range byKey {
		preview.Disable = append(preview.Disable, e)
	}
	sort.Slice(preview.Disable, func(i, j int) bool { return preview.Disable[i].SKU.ID < preview.Disable[j].SKU.ID })
	return preview, nil
}

func (s *Service) PreviewSKUMatrix(ctx context.Context, userID, productID uint, input SKUMatrixInput) (SKUMatrixPreview, error) {
	return s.previewWithDB(ctx, s.repository.db, userID, productID, input)
}

func (s *Service) GetSKUConfig(ctx context.Context, userID, productID uint) (SKUConfig, error) {
	return s.getSKUConfigWithDB(ctx, s.repository.db, userID, productID)
}
func (s *Service) getSKUConfigWithDB(ctx context.Context, db *gorm.DB, userID, productID uint) (SKUConfig, error) {
	p, err := s.ownedProduct(ctx, userID, productID, db)
	if err != nil {
		return SKUConfig{}, err
	}
	var d []CommerceSKUDimension
	var v []CommerceSKUValue
	if err := db.WithContext(ctx).Where("user_id=? AND product_id=?", userID, productID).Order("sort_order").Find(&d).Error; err != nil {
		return SKUConfig{}, err
	}
	if err := db.WithContext(ctx).Where("user_id=? AND product_id=?", userID, productID).Order("sort_order").Find(&v).Error; err != nil {
		return SKUConfig{}, err
	}
	_, skus, err := s.existingCandidates(ctx, db, userID, productID)
	if err != nil {
		return SKUConfig{}, err
	}
	var projects []CommerceProject
	if err := db.WithContext(ctx).Where("user_id=? AND product_id=?", userID, productID).Order("id").Find(&projects).Error; err != nil {
		return SKUConfig{}, err
	}
	var defaultID uint
	defaultKnown := false
	for _, project := range projects {
		if project.DefaultSKUID == nil {
			continue
		}
		if !defaultKnown {
			defaultID = *project.DefaultSKUID
			defaultKnown = true
		} else if defaultID != *project.DefaultSKUID {
			defaultID = 0
			defaultKnown = false
			break
		}
	}
	return SKUConfig{Version: p.SKUVersion, DefaultSKUID: defaultID, DefaultKnown: defaultKnown, Dimensions: d, Values: v, SKUs: skus}, nil
}

func (s *Service) ApplySKUMatrix(ctx context.Context, userID, productID uint, key string, input SKUMatrixInput) (SKUConfig, error) {
	s.skuMutationMu.Lock()
	defer s.skuMutationMu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return SKUConfig{}, invalidField("Idempotency-Key", "必须提供幂等键")
	}
	normalized, err := normalizeMatrix(input)
	if err != nil {
		return SKUConfig{}, err
	}
	digest := digestMatrix(normalized)
	var out SKUConfig
	err = s.repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p CommerceProduct
		findProduct := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", productID).First(&p).Error
		if findProduct != nil {
			return mapNotFound(findProduct)
		}
		if p.UserID != userID {
			return ErrOwnershipMismatch
		}

		claim := CommerceSKUMatrixRequest{UserID: userID, ProductID: productID, IdempotencyKey: key, RequestDigest: digest, ResponseJSON: ""}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "product_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&claim)
		if created.Error != nil {
			return created.Error
		}
		var record CommerceSKUMatrixRequest
		if created.RowsAffected == 0 {
			find := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND product_id=? AND idempotency_key=?", userID, productID, key).First(&record).Error
			if find != nil {
				return find
			}
			if record.RequestDigest != digest {
				return ErrIdempotencyConflict
			}
			if record.ResponseJSON == "" {
				return fmt.Errorf("SKU matrix idempotency response is pending")
			}
			return json.Unmarshal([]byte(record.ResponseJSON), &out)
		}
		cas := tx.Model(&CommerceProduct{}).Where("id=? AND user_id=? AND sku_version=?", productID, userID, input.ExpectedVersion).Update("sku_version", input.ExpectedVersion+1)
		if cas.Error != nil {
			return cas.Error
		}
		if cas.RowsAffected != 1 {
			return ErrSKUVersionConflict
		}
		p.SKUVersion = input.ExpectedVersion
		var lockedProjects []CommerceProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND product_id=?", userID, productID).Find(&lockedProjects).Error; err != nil {
			return err
		}
		preview, err := s.previewWithDB(ctx, tx, userID, productID, normalized)
		if err != nil {
			return err
		}
		if len(preview.Conflicts) > 0 {
			return ErrConflict
		}
		disabledIDs := make([]uint, 0, len(preview.Disable))
		for _, candidate := range preview.Disable {
			disabledIDs = append(disabledIDs, candidate.SKU.ID)
		}
		if err := tx.Model(&CommerceSKUDimension{}).Where("user_id=? AND product_id=?", userID, productID).Update("status", "disabled").Error; err != nil {
			return err
		}
		if err := tx.Model(&CommerceSKUValue{}).Where("user_id=? AND product_id=?", userID, productID).Update("status", "disabled").Error; err != nil {
			return err
		}
		valueIDs := map[string]uint{}
		for di, d := range normalized.Dimensions {
			var dm CommerceSKUDimension
			err := tx.Where("user_id=? AND product_id=? AND name=?", userID, productID, d.Name).First(&dm).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				dm = CommerceSKUDimension{UserID: userID, ProductID: productID, Name: d.Name, Version: p.SKUVersion + 1, SortOrder: di, Status: "active"}
				err = tx.Create(&dm).Error
			} else if err == nil {
				err = tx.Model(&dm).Updates(map[string]any{"version": p.SKUVersion + 1, "sort_order": di, "status": "active"}).Error
			}
			if err != nil {
				return err
			}
			for vi, v := range d.Values {
				var vm CommerceSKUValue
				err := tx.Where("dimension_id=? AND name=?", dm.ID, v.Name).First(&vm).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					vm = CommerceSKUValue{UserID: userID, ProductID: productID, DimensionID: dm.ID, Name: v.Name, SortOrder: vi, Status: "active"}
					err = tx.Create(&vm).Error
				} else if err == nil {
					err = tx.Model(&vm).Updates(map[string]any{"sort_order": vi, "status": "active"}).Error
				}
				if err != nil {
					return err
				}
				valueIDs[d.Name+"="+v.Name] = vm.ID
			}
		}
		for _, c := range preview.Disable {
			if err := tx.Model(&CommerceSKU{}).Where("id=?", c.SKU.ID).Update("status", "disabled").Error; err != nil {
				return err
			}
		}
		sequence := 1
		var firstActiveSKUID uint
		for _, c := range append(preview.Keep, preview.Add...) {
			var sku CommerceSKU
			if c.SKU != nil {
				sku = *c.SKU
				if err := tx.Model(&CommerceSKU{}).Where("id=?", sku.ID).Update("status", "active").Error; err != nil {
					return err
				}
			} else {
				code := c.Code
				if code == "" {
					for {
						code = fmt.Sprintf("SKU-%d-%d", productID, sequence)
						sequence++
						var n int64
						if err := tx.Model(&CommerceSKU{}).Where("product_id=? AND code=?", productID, code).Count(&n).Error; err != nil {
							return err
						}
						if n == 0 {
							break
						}
					}
				}
				sku = CommerceSKU{UserID: userID, ProductID: productID, Code: code, Status: "active", AttributesJSON: "{}"}
				if err := tx.Create(&sku).Error; err != nil {
					if isUniqueConstraintError(err) {
						return ErrConflict
					}
					return err
				}
			}
			if firstActiveSKUID == 0 {
				firstActiveSKUID = sku.ID
			}
			if err := tx.Where("sku_id=?", sku.ID).Delete(&CommerceSKUValueLink{}).Error; err != nil {
				return err
			}
			for _, spec := range c.Specifications {
				link := CommerceSKUValueLink{UserID: userID, ProductID: productID, SKUID: sku.ID, ValueID: valueIDs[spec.Dimension+"="+spec.Value]}
				if err := tx.Create(&link).Error; err != nil {
					return err
				}
			}
		}
		if len(disabledIDs) > 0 {
			var referenced int64
			if err := tx.Model(&CommerceProject{}).Where("user_id=? AND product_id=? AND default_sku_id IN ?", userID, productID, disabledIDs).Count(&referenced).Error; err != nil {
				return err
			}
			if referenced > 0 {
				if firstActiveSKUID == 0 {
					return ErrDefaultSKUDisable
				}
				if err := tx.Model(&CommerceProject{}).Where("user_id=? AND product_id=? AND default_sku_id IN ?", userID, productID, disabledIDs).Update("default_sku_id", firstActiveSKUID).Error; err != nil {
					return err
				}
			}
		}
		out, err = s.getSKUConfigWithDB(ctx, tx, userID, productID)
		if err != nil {
			return err
		}
		raw, marshalErr := json.Marshal(out)
		if marshalErr != nil {
			return marshalErr
		}
		result := tx.Model(&CommerceSKUMatrixRequest{}).Where("id=? AND request_digest=?", claim.ID, digest).Update("response_json", string(raw))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrIdempotencyConflict
		}
		return nil
	})
	return out, err
}
