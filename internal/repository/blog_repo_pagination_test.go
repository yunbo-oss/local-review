package repository

import (
	"context"
	"testing"
	"time"

	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestBlogRepoListByShopIDPageUsesStableCursor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:blog-page?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Blog{}); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)
	blogs := []model.Blog{
		{ShopId: 7, Title: "A", Liked: 10, CreateTime: base.Add(time.Minute), UpdateTime: base.Add(time.Minute)},
		{ShopId: 7, Title: "B", Liked: 8, CreateTime: base.Add(2 * time.Minute), UpdateTime: base.Add(2 * time.Minute)},
		{ShopId: 7, Title: "C", Liked: 5, CreateTime: base.Add(3 * time.Minute), UpdateTime: base.Add(3 * time.Minute)},
		{ShopId: 9, Title: "other", Liked: 99, CreateTime: base, UpdateTime: base},
	}
	if err := db.Create(&blogs).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewBlogRepo(db)
	paged, ok := repo.(repoInterfaces.PaginatedBlogRepo)
	if !ok {
		t.Fatal("blog repository does not implement pagination")
	}
	first, err := paged.ListByShopIDPage(context.Background(), repoInterfaces.BlogPageRequest{
		ShopID: 7, Limit: 2, Sort: "liked",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Blogs) != 2 || first.Blogs[0].Title != "A" || first.Blogs[1].Title != "B" || first.NextCursor == "" {
		t.Fatalf("first page=%+v", first)
	}
	second, err := paged.ListByShopIDPage(context.Background(), repoInterfaces.BlogPageRequest{
		ShopID: 7, Limit: 2, Sort: "liked", Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Blogs) != 1 || second.Blogs[0].Title != "C" {
		t.Fatalf("second page=%+v", second)
	}
}

func TestBlogRepoListByShopIDPageRejectsCursorFromDifferentSort(t *testing.T) {
	cursor := encodeBlogPageCursor(blogPageCursor{Sort: "liked", Liked: 5, ID: 3})
	if _, err := decodeBlogPageCursor(cursor, "recent"); err == nil {
		t.Fatal("cursor sort mismatch should be rejected")
	}
}
