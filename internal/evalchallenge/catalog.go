package evalchallenge

import "fmt"

type catalogShop struct {
	ID       int64
	Name     string
	TypeName string
	Area     string
	Price    int64
	Score    int
	Comments int
	Hours    string
	Theme    string
}

var (
	areas  = []string{"朝阳区", "海淀区", "西城区", "东城区", "丰台区"}
	types  = []string{"美食", "咖啡", "酒店", "烘焙", "日料", "健身", "亲子", "书店"}
	themes = []string{"浪漫约会", "安静办公", "家庭聚餐", "深夜营业", "宠物友好", "无障碍", "商务宴请", "学生平价"}
)

// catalog reproduces only the immutable fields needed to grade the 200-shop
// eval-data.v2 catalog. The source manifest hash is checked before artifacts
// are written, so a future catalog change cannot silently produce stale goldens.
func catalog() []catalogShop {
	base := []struct {
		id, typeID, price int64
		area              string
		score, comments   int
	}{
		{1, 1, 35, "朝阳区", 48, 320}, {2, 1, 45, "海淀区", 46, 180},
		{3, 2, 55, "西城区", 50, 500}, {4, 2, 25, "东城区", 47, 800},
		{5, 1, 120, "朝阳区", 49, 1200}, {6, 3, 200, "海淀区", 45, 150},
		{7, 1, 80, "朝阳区", 48, 400}, {8, 2, 30, "海淀区", 46, 350},
		{9, 1, 150, "东城区", 49, 600}, {10, 3, 180, "丰台区", 44, 80},
		{11, 1, 90, "朝阳区", 48, 550}, {12, 2, 28, "海淀区", 47, 900},
		{13, 1, 65, "西城区", 46, 700}, {14, 3, 150, "东城区", 43, 90},
		{15, 1, 75, "朝阳区", 47, 380}, {16, 2, 45, "西城区", 46, 280},
		{17, 1, 110, "海淀区", 48, 220}, {18, 2, 35, "朝阳区", 47, 750},
		{19, 1, 95, "东城区", 48, 360}, {20, 3, 350, "朝阳区", 49, 85},
		{21, 1, 85, "海淀区", 47, 290}, {22, 2, 38, "丰台区", 46, 180},
		{23, 1, 100, "朝阳区", 48, 450}, {24, 2, 12, "西城区", 45, 1200},
		{25, 1, 130, "东城区", 50, 800},
	}
	out := make([]catalogShop, 0, 200)
	for _, s := range base {
		out = append(out, catalogShop{
			ID: s.id, Name: fmt.Sprintf("base-shop-%02d", s.id), TypeName: types[s.typeID-1],
			Area: s.area, Price: s.price, Score: s.score, Comments: s.comments,
		})
	}

	prefixes := []string{"拾光", "云栖", "青禾", "北辰", "木棉", "知味", "星桥", "南风", "山海", "晴川"}
	suffixes := []string{"小馆", "空间", "生活馆", "工坊", "会客厅"}
	for id := int64(26); id <= 200; id++ {
		i := int(id - 26)
		typeName := types[i%len(types)]
		name := fmt.Sprintf("%s%s%s%03d", prefixes[(i/2)%len(prefixes)], typeName, suffixes[(i/10)%len(suffixes)], i+1)
		if i%20 == 0 {
			name = fmt.Sprintf("双子%s旗舰店%02d", typeName, i/20+1)
		} else if i%20 == 1 {
			name = fmt.Sprintf("双子%s旗舰店%02d·近邻店", typeName, i/20+1)
		}
		out = append(out, catalogShop{
			ID: id, Name: name, TypeName: typeName, Area: areas[i%len(areas)],
			Price: int64(18 + (i*17)%333), Score: 38 + (i*3)%13,
			Comments: 30 + (i*47)%1970,
			Hours:    []string{"07:00-21:00", "09:00-22:00", "10:00-24:00", "18:00-02:00"}[i%4],
			Theme:    themes[(i/len(types))%len(themes)],
		})
	}
	// These five anchors are intentionally frozen by generate-eval-data.
	patch := func(id int64, name, area, typeName string, price int64, theme string) {
		s := &out[id-1]
		s.Name, s.Area, s.TypeName, s.Price, s.Theme = name, area, typeName, price, theme
	}
	patch(26, "静巷咖啡·国贸店", "朝阳区", "咖啡", 42, "安静办公")
	patch(27, "静巷咖啡·望京店", "朝阳区", "咖啡", 98, "浪漫约会")
	patch(28, "静巷咖啡·五道口店", "海淀区", "咖啡", 36, "学生平价")
	patch(29, "无界餐厅", "东城区", "美食", 128, "无障碍")
	patch(30, "月下餐厅", "朝阳区", "美食", 168, "浪漫约会")
	return out
}

func selectShops(shops []catalogShop, limit int, pred func(catalogShop) bool) []int64 {
	out := make([]int64, 0)
	for _, s := range shops {
		if !pred(s) {
			continue
		}
		out = append(out, s.ID)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func forbiddenNearMisses(shops []catalogShop, allowed []int64, area, typeName, theme string, maxPrice int64) []int64 {
	allowedSet := make(map[int64]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	out := make([]int64, 0, 8)
	for _, s := range shops {
		if _, ok := allowedSet[s.ID]; ok {
			continue
		}
		sharesIntent := (area != "" && s.Area == area) || (typeName != "" && s.TypeName == typeName) || (theme != "" && s.Theme == theme)
		violates := (area != "" && s.Area != area) || (typeName != "" && s.TypeName != typeName) ||
			(theme != "" && s.Theme != theme) || (maxPrice > 0 && s.Price > maxPrice)
		if sharesIntent && violates {
			out = append(out, s.ID)
			if len(out) == 8 {
				break
			}
		}
	}
	return out
}
