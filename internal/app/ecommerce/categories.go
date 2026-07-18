package ecommerce

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	CategoryCatalogVersion = "cn-commerce-v1"
	CategoryStatusActive   = "active"
	CategoryStatusInactive = "inactive"
	CategorySourceSystem   = "system"
	CategorySourceUser     = "user"
)

var (
	ErrCategoryConflict    = errors.New("commerce category already exists")
	ErrCategoryUnavailable = errors.New("commerce category unavailable")
	categorySpaces         = regexp.MustCompile(`\s+`)
)

type CategoryNode struct {
	ID, ParentID       uint
	Source             string
	Name, Path, Status string
	Aliases            []string
	SortOrder          int
	Children           []CategoryNode
}

type CategoryCatalog struct {
	Version          string
	SystemCategories []CategoryNode
	CustomCategories []CategoryNode
	RecentCategories []CategoryNode
}

type CreateCustomCategoryInput struct {
	ParentID uint
	Name     string
}
type PatchCustomCategoryInput struct{ Name, Status *string }

type CreateSystemCategoryInput struct {
	ParentID  *uint
	Level     int
	Name      string
	Aliases   []string
	SortOrder int
}
type PatchSystemCategoryInput struct {
	Name, Status *string
	Aliases      *[]string
	SortOrder    *int
}

type categorySeed struct {
	Name     string
	Aliases  []string
	Children []categorySeed
}

var defaultCategorySeeds = []categorySeed{
	{"服饰鞋包", nil, []categorySeed{{"女装", nil, nil}, {"男装", nil, nil}, {"内衣", nil, nil}, {"童装", nil, nil}, {"鞋靴", nil, nil}, {"箱包", nil, nil}, {"服饰配件", nil, nil}}},
	{"美妆个护", nil, []categorySeed{{"护肤", []string{"面霜", "精华", "面膜"}, nil}, {"彩妆", nil, nil}, {"香水", nil, nil}, {"洗发护发", nil, nil}, {"身体护理", nil, nil}, {"口腔护理", nil, nil}, {"美容工具", nil, nil}}},
	{"食品饮料", nil, []categorySeed{{"休闲零食", nil, nil}, {"粮油调味", nil, nil}, {"茶饮冲调", nil, nil}, {"酒类", nil, nil}, {"乳品", nil, nil}, {"生鲜", nil, nil}, {"营养食品", nil, nil}}},
	{"家居日用", []string{"家居百货"}, []categorySeed{{"杯壶餐具", []string{"水杯", "保温杯", "杯子", "餐具"}, nil}, {"收纳整理", nil, nil}, {"清洁用品", nil, nil}, {"厨具", nil, nil}, {"床上用品", nil, nil}, {"家纺", nil, nil}, {"卫浴用品", nil, nil}}},
	{"家装建材", nil, []categorySeed{{"家具", nil, nil}, {"灯具", nil, nil}, {"五金工具", nil, nil}, {"厨卫建材", nil, nil}, {"墙地面材料", nil, nil}, {"全屋智能", nil, nil}, {"装饰摆件", nil, nil}}},
	{"数码电器", []string{"3C数码"}, []categorySeed{{"手机及配件", nil, nil}, {"电脑办公", nil, nil}, {"摄影摄像", nil, nil}, {"影音娱乐", nil, nil}, {"厨房电器", nil, nil}, {"生活电器", nil, nil}, {"大家电", nil, nil}}},
	{"母婴童装", nil, []categorySeed{{"奶粉辅食", nil, nil}, {"尿裤湿巾", nil, nil}, {"喂养用品", nil, nil}, {"洗护用品", nil, nil}, {"婴童用品", nil, nil}, {"孕产用品", nil, nil}, {"童装童鞋", nil, nil}}},
	{"运动户外", nil, []categorySeed{{"运动服饰", nil, nil}, {"运动鞋", nil, nil}, {"健身器材", nil, nil}, {"露营装备", nil, nil}, {"骑行", nil, nil}, {"球类", nil, nil}, {"垂钓", nil, nil}, {"户外用品", nil, nil}}},
	{"珠宝配饰", nil, []categorySeed{{"黄金", nil, nil}, {"珠宝玉石", nil, nil}, {"时尚饰品", nil, nil}, {"眼镜", nil, nil}, {"钟表", nil, nil}, {"发饰", nil, nil}}},
	{"宠物用品", nil, []categorySeed{{"宠物食品", nil, nil}, {"宠物清洁", nil, nil}, {"宠物服饰", nil, nil}, {"宠物玩具", nil, nil}, {"宠物出行", nil, nil}, {"水族用品", nil, nil}}},
	{"汽车用品", nil, []categorySeed{{"汽车内饰", nil, nil}, {"汽车外饰", nil, nil}, {"清洁养护", nil, nil}, {"电子电器", nil, nil}, {"安全应急", nil, nil}, {"摩托车用品", nil, nil}}},
	{"办公文教", nil, []categorySeed{{"文具", nil, nil}, {"办公用品", nil, nil}, {"学生用品", nil, nil}, {"图书", nil, nil}, {"绘画用品", nil, nil}, {"打印耗材", nil, nil}}},
	{"玩具乐器", nil, []categorySeed{{"益智玩具", nil, nil}, {"模型动漫", nil, nil}, {"毛绒玩具", nil, nil}, {"儿童玩具", nil, nil}, {"乐器", nil, nil}, {"棋牌娱乐", nil, nil}}},
	{"医药保健", nil, []categorySeed{{"保健器械", nil, nil}, {"健康监测", nil, nil}, {"护理用品", nil, nil}, {"成人保健", nil, nil}, {"传统滋补", nil, nil}}},
	{"本地生活/虚拟服务", []string{"本地生活", "虚拟服务"}, []categorySeed{{"餐饮服务", nil, nil}, {"旅游出行", nil, nil}, {"教育培训", nil, nil}, {"生活服务", nil, nil}, {"数字商品", nil, nil}, {"软件服务", nil, nil}}},
	{"其他", nil, []categorySeed{{"工业品", nil, nil}, {"农资园艺", nil, nil}, {"定制商品", nil, nil}, {"礼品", nil, nil}, {"暂未分类", nil, nil}}},
}

func SeedDefaultCategories(ctx context.Context, db *gorm.DB) error {
	for index, root := range defaultCategorySeeds {
		rootKey := fmt.Sprintf("root-%02d", index+1)
		row := CommerceSystemCategory{Level: 1, Name: root.Name, SeedKey: rootKey, SearchAliasesJSON: mustCategoryAliases(root.Aliases), SortOrder: (index + 1) * 10, Status: CategoryStatusActive, CatalogVersion: CategoryCatalogVersion}
		if err := db.WithContext(ctx).Where("seed_key = ?", rootKey).FirstOrCreate(&row).Error; err != nil {
			return err
		}
		for childIndex, child := range root.Children {
			childKey := fmt.Sprintf("%s-child-%02d", rootKey, childIndex+1)
			childRow := CommerceSystemCategory{ParentID: &row.ID, Level: 2, Name: child.Name, SeedKey: childKey, SearchAliasesJSON: mustCategoryAliases(child.Aliases), SortOrder: (childIndex + 1) * 10, Status: CategoryStatusActive, CatalogVersion: CategoryCatalogVersion}
			if err := db.WithContext(ctx).Where("seed_key = ?", childKey).FirstOrCreate(&childRow).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func mustCategoryAliases(values []string) string { raw, _ := EncodeJSON(values); return raw }
func normalizeCategoryName(value string) string {
	return categorySpaces.ReplaceAllString(strings.TrimSpace(value), " ")
}
func validCategoryName(value string) bool {
	length := len([]rune(value))
	return length >= 2 && length <= 20
}

func (s *Service) ListCategories(ctx context.Context, userID uint) (CategoryCatalog, error) {
	var system []CommerceSystemCategory
	if err := s.repository.DB().WithContext(ctx).Where("status = ?", CategoryStatusActive).Order("level, sort_order, id").Find(&system).Error; err != nil {
		return CategoryCatalog{}, err
	}
	roots, rootByID := make([]CategoryNode, 0), map[uint]int{}
	for _, row := range system {
		if row.Level == 1 {
			rootByID[row.ID] = len(roots)
			roots = append(roots, systemCategoryNode(row, ""))
		}
	}
	for _, row := range system {
		if row.Level == 2 && row.ParentID != nil {
			if index, ok := rootByID[*row.ParentID]; ok {
				node := systemCategoryNode(row, roots[index].Name)
				roots[index].Children = append(roots[index].Children, node)
			}
		}
	}
	var customRows []CommerceUserCategory
	if err := s.repository.DB().WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC, id DESC").Find(&customRows).Error; err != nil {
		return CategoryCatalog{}, err
	}
	custom := make([]CategoryNode, 0, len(customRows))
	for _, row := range customRows {
		parentName := ""
		if index, ok := rootByID[row.ParentID]; ok {
			parentName = roots[index].Name
		}
		custom = append(custom, userCategoryNode(row, parentName))
	}
	recent, err := s.recentCategoryNodes(ctx, userID, system, customRows)
	if err != nil {
		return CategoryCatalog{}, err
	}
	return CategoryCatalog{Version: CategoryCatalogVersion, SystemCategories: roots, CustomCategories: custom, RecentCategories: recent}, nil
}

func systemCategoryNode(row CommerceSystemCategory, parent string) CategoryNode {
	var aliases []string
	_ = DecodeJSON(row.SearchAliasesJSON, &aliases)
	path := row.Name
	if parent != "" {
		path = parent + " / " + row.Name
	}
	parentID := uint(0)
	if row.ParentID != nil {
		parentID = *row.ParentID
	}
	return CategoryNode{ID: row.ID, ParentID: parentID, Source: CategorySourceSystem, Name: row.Name, Path: path, Status: row.Status, Aliases: aliases, SortOrder: row.SortOrder}
}
func userCategoryNode(row CommerceUserCategory, parent string) CategoryNode {
	var aliases []string
	_ = DecodeJSON(row.SearchAliasesJSON, &aliases)
	return CategoryNode{ID: row.ID, ParentID: row.ParentID, Source: CategorySourceUser, Name: row.Name, Path: parent + " / " + row.Name, Status: row.Status, Aliases: aliases}
}

func (s *Service) recentCategoryNodes(ctx context.Context, userID uint, system []CommerceSystemCategory, custom []CommerceUserCategory) ([]CategoryNode, error) {
	var products []CommerceProduct
	if err := s.repository.DB().WithContext(ctx).Where("user_id = ? AND category_id IS NOT NULL", userID).Order("created_at DESC").Limit(30).Find(&products).Error; err != nil {
		return nil, err
	}
	systemByID := map[uint]CommerceSystemCategory{}
	for _, row := range system {
		systemByID[row.ID] = row
	}
	customByID := map[uint]CommerceUserCategory{}
	for _, row := range custom {
		customByID[row.ID] = row
	}
	seen := map[string]bool{}
	result := make([]CategoryNode, 0, 5)
	for _, product := range products {
		if product.CategoryID == nil {
			continue
		}
		key := product.CategorySource + ":" + strconv.FormatUint(uint64(*product.CategoryID), 10)
		if seen[key] {
			continue
		}
		var node CategoryNode
		var ok bool
		if product.CategorySource == CategorySourceUser {
			row, found := customByID[*product.CategoryID]
			if found && row.Status == CategoryStatusActive {
				node, ok = userCategoryNode(row, strings.Split(product.CategoryPath, " / ")[0]), true
			}
		} else {
			row, found := systemByID[*product.CategoryID]
			if found {
				parent := ""
				if row.ParentID != nil {
					parent = systemByID[*row.ParentID].Name
				}
				node, ok = systemCategoryNode(row, parent), true
			}
		}
		if ok {
			seen[key] = true
			result = append(result, node)
			if len(result) == 5 {
				break
			}
		}
	}
	return result, nil
}

func (s *Service) CreateCustomCategory(ctx context.Context, userID uint, input CreateCustomCategoryInput) (CommerceUserCategory, error) {
	name := normalizeCategoryName(input.Name)
	if !validCategoryName(name) {
		return CommerceUserCategory{}, invalidField("name", "品类名称长度必须为 2 到 20 个字符")
	}
	var parent CommerceSystemCategory
	if err := s.repository.DB().WithContext(ctx).Where("id = ? AND level = 1 AND status = ?", input.ParentID, CategoryStatusActive).First(&parent).Error; err != nil {
		return CommerceUserCategory{}, ErrCategoryUnavailable
	}
	var count int64
	db := s.repository.DB().WithContext(ctx)
	db.Model(&CommerceUserCategory{}).Where("user_id = ? AND parent_id = ? AND name = ?", userID, input.ParentID, name).Count(&count)
	if count > 0 {
		return CommerceUserCategory{}, ErrCategoryConflict
	}
	db.Model(&CommerceSystemCategory{}).Where("parent_id = ? AND name = ?", input.ParentID, name).Count(&count)
	if count > 0 {
		return CommerceUserCategory{}, ErrCategoryConflict
	}
	row := CommerceUserCategory{UserID: userID, ParentID: input.ParentID, Name: name, SearchAliasesJSON: "[]", Status: CategoryStatusActive}
	if err := db.Create(&row).Error; err != nil {
		return CommerceUserCategory{}, err
	}
	return row, nil
}

func (s *Service) PatchCustomCategory(ctx context.Context, userID, id uint, input PatchCustomCategoryInput) (CommerceUserCategory, error) {
	db := s.repository.DB().WithContext(ctx)
	var row CommerceUserCategory
	if err := db.First(&row, id).Error; err != nil {
		return CommerceUserCategory{}, mapNotFound(err)
	}
	if row.UserID != userID {
		return CommerceUserCategory{}, ErrOwnershipMismatch
	}
	if input.Name != nil {
		name := normalizeCategoryName(*input.Name)
		if !validCategoryName(name) {
			return CommerceUserCategory{}, invalidField("name", "品类名称长度必须为 2 到 20 个字符")
		}
		var count int64
		db.Model(&CommerceUserCategory{}).Where("user_id = ? AND parent_id = ? AND name = ? AND id <> ?", userID, row.ParentID, name, row.ID).Count(&count)
		if count > 0 {
			return CommerceUserCategory{}, ErrCategoryConflict
		}
		db.Model(&CommerceSystemCategory{}).Where("parent_id = ? AND name = ?", row.ParentID, name).Count(&count)
		if count > 0 {
			return CommerceUserCategory{}, ErrCategoryConflict
		}
		row.Name = name
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != CategoryStatusActive && status != CategoryStatusInactive {
			return CommerceUserCategory{}, invalidField("status", "品类状态无效")
		}
		row.Status = status
	}
	if err := db.Save(&row).Error; err != nil {
		return CommerceUserCategory{}, err
	}
	return row, nil
}

func (s *Service) ResolveCategorySelection(ctx context.Context, userID, id uint, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == CategorySourceSystem {
		var child CommerceSystemCategory
		if err := s.repository.DB().WithContext(ctx).Where("id = ? AND level = 2 AND status = ?", id, CategoryStatusActive).First(&child).Error; err != nil {
			return "", ErrCategoryUnavailable
		}
		var parent CommerceSystemCategory
		if child.ParentID == nil || s.repository.DB().WithContext(ctx).Where("id = ? AND status = ?", *child.ParentID, CategoryStatusActive).First(&parent).Error != nil {
			return "", ErrCategoryUnavailable
		}
		return parent.Name + " / " + child.Name, nil
	}
	if source == CategorySourceUser {
		var custom CommerceUserCategory
		if err := s.repository.DB().WithContext(ctx).Where("id = ? AND user_id = ? AND status = ?", id, userID, CategoryStatusActive).First(&custom).Error; err != nil {
			return "", ErrCategoryUnavailable
		}
		var parent CommerceSystemCategory
		if s.repository.DB().WithContext(ctx).Where("id = ? AND status = ?", custom.ParentID, CategoryStatusActive).First(&parent).Error != nil {
			return "", ErrCategoryUnavailable
		}
		return parent.Name + " / " + custom.Name, nil
	}
	return "", ErrCategoryUnavailable
}

func (s *Service) ListAdminCategories(ctx context.Context) ([]CommerceSystemCategory, error) {
	var rows []CommerceSystemCategory
	err := s.repository.DB().WithContext(ctx).Order("level, sort_order, id").Find(&rows).Error
	return rows, err
}

func (s *Service) CountUserCategories(ctx context.Context) (int64, error) {
	var count int64
	err := s.repository.DB().WithContext(ctx).Model(&CommerceUserCategory{}).Count(&count).Error
	return count, err
}

func (s *Service) CreateSystemCategory(ctx context.Context, input CreateSystemCategoryInput) (CommerceSystemCategory, error) {
	name := normalizeCategoryName(input.Name)
	if !validCategoryName(name) || (input.Level != 1 && input.Level != 2) {
		return CommerceSystemCategory{}, invalidField("category", "商品品类无效")
	}
	if input.Level == 1 {
		input.ParentID = nil
	} else {
		if input.ParentID == nil {
			return CommerceSystemCategory{}, invalidField("parent_id", "请选择所属一级品类")
		}
		var parent CommerceSystemCategory
		if s.repository.DB().WithContext(ctx).Where("id = ? AND level = 1", *input.ParentID).First(&parent).Error != nil {
			return CommerceSystemCategory{}, ErrCategoryUnavailable
		}
	}
	var count int64
	query := s.repository.DB().WithContext(ctx).Model(&CommerceSystemCategory{}).Where("name = ?", name)
	if input.ParentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *input.ParentID)
	}
	query.Count(&count)
	if count > 0 {
		return CommerceSystemCategory{}, ErrCategoryConflict
	}
	row := CommerceSystemCategory{ParentID: input.ParentID, Level: input.Level, Name: name, SearchAliasesJSON: mustCategoryAliases(input.Aliases), SortOrder: input.SortOrder, Status: CategoryStatusActive, CatalogVersion: CategoryCatalogVersion}
	if err := s.repository.DB().WithContext(ctx).Create(&row).Error; err != nil {
		return CommerceSystemCategory{}, err
	}
	return row, nil
}

func (s *Service) PatchSystemCategory(ctx context.Context, id uint, input PatchSystemCategoryInput) (CommerceSystemCategory, error) {
	db := s.repository.DB().WithContext(ctx)
	var row CommerceSystemCategory
	if err := db.First(&row, id).Error; err != nil {
		return CommerceSystemCategory{}, mapNotFound(err)
	}
	if input.Name != nil {
		name := normalizeCategoryName(*input.Name)
		if !validCategoryName(name) {
			return CommerceSystemCategory{}, invalidField("name", "品类名称长度必须为 2 到 20 个字符")
		}
		var count int64
		query := db.Model(&CommerceSystemCategory{}).Where("name = ? AND id <> ?", name, row.ID)
		if row.ParentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *row.ParentID)
		}
		if err := query.Count(&count).Error; err != nil {
			return CommerceSystemCategory{}, err
		}
		if count > 0 {
			return CommerceSystemCategory{}, ErrCategoryConflict
		}
		row.Name = name
	}
	if input.Aliases != nil {
		row.SearchAliasesJSON = mustCategoryAliases(*input.Aliases)
	}
	if input.SortOrder != nil {
		row.SortOrder = *input.SortOrder
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != CategoryStatusActive && status != CategoryStatusInactive {
			return CommerceSystemCategory{}, invalidField("status", "品类状态无效")
		}
		if status == CategoryStatusInactive && row.Level == 1 {
			var activeChildren int64
			db.Model(&CommerceSystemCategory{}).Where("parent_id = ? AND status = ?", row.ID, CategoryStatusActive).Count(&activeChildren)
			if activeChildren > 0 {
				return CommerceSystemCategory{}, ErrCategoryConflict
			}
		}
		row.Status = status
	}
	if err := db.Save(&row).Error; err != nil {
		return CommerceSystemCategory{}, err
	}
	return row, nil
}
