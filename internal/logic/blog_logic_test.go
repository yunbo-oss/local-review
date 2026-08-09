package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"
)

type blogRepoStub struct {
	repoInterfaces.BlogRepo
	created *model.Blog
	id      int64
	shopID  int64
	likes   int
}

func (s *blogRepoStub) IncrLike(context.Context, int64) error {
	s.likes++
	return nil
}

func (s *blogRepoStub) GetByID(context.Context, int64) (*model.Blog, error) {
	return &model.Blog{Id: s.id, ShopId: s.shopID}, nil
}

func (s *blogRepoStub) Create(_ context.Context, blog *model.Blog) (int64, error) {
	s.created = blog
	blog.Id = s.id
	return s.id, nil
}

type followRepoStub struct {
	repoInterfaces.FollowRepo
	err error
}

func (s *followRepoStub) ListByFollowUserID(context.Context, int64) ([]model.Follow, error) {
	return nil, s.err
}

type shopUpdateProducerStub struct {
	shopIDs []int64
	err     error
}

func (s *shopUpdateProducerStub) SendShopUpdate(_ context.Context, shopID int64) error {
	s.shopIDs = append(s.shopIDs, shopID)
	return s.err
}

func TestSaveBlogReturnsCommittedIDAndRefreshesVectorWithoutFollowers(t *testing.T) {
	blogRepo := &blogRepoStub{id: 42}
	followRepo := &followRepoStub{}
	producer := &shopUpdateProducerStub{}
	logic := NewBlogLogic(BlogLogicDeps{
		BlogRepo:           blogRepo,
		UserRepo:           userRepoStub{},
		FollowRepo:         followRepo,
		ShopUpdateProducer: producer,
	})

	blog := &model.Blog{ShopId: 7, UserId: 999, Title: "探店"}
	id, err := logic.SaveBlog(context.Background(), 12, blog)
	if err != nil {
		t.Fatalf("SaveBlog() error = %v", err)
	}
	if id != 42 {
		t.Fatalf("SaveBlog() id = %d, want 42", id)
	}
	if blogRepo.created.UserId != 12 {
		t.Fatalf("stored user id = %d, want authenticated user 12", blogRepo.created.UserId)
	}
	if len(producer.shopIDs) != 1 || producer.shopIDs[0] != 7 {
		t.Fatalf("vector refresh shops = %v, want [7]", producer.shopIDs)
	}
}

func TestSaveBlogDoesNotReportFailureAfterCommitWhenDerivedUpdatesFail(t *testing.T) {
	blogRepo := &blogRepoStub{id: 43}
	producer := &shopUpdateProducerStub{err: errors.New("mq unavailable")}
	logic := NewBlogLogic(BlogLogicDeps{
		BlogRepo:           blogRepo,
		UserRepo:           userRepoStub{},
		FollowRepo:         &followRepoStub{err: errors.New("followers unavailable")},
		ShopUpdateProducer: producer,
	})

	id, err := logic.SaveBlog(context.Background(), 12, &model.Blog{ShopId: 8})
	if err != nil {
		t.Fatalf("SaveBlog() returned an error after DB commit: %v", err)
	}
	if id != 43 {
		t.Fatalf("SaveBlog() id = %d, want committed id 43", id)
	}
}

type userRepoStub struct{ repoInterfaces.UserRepo }

func TestLikeBlogRefreshesVectorBecauseTopReviewsMayChange(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	redisCli := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = redisCli.Close() })

	blogRepo := &blogRepoStub{id: 51, shopID: 29}
	producer := &shopUpdateProducerStub{}
	logic := NewBlogLogic(BlogLogicDeps{
		BlogRepo: blogRepo, UserRepo: userRepoStub{}, FollowRepo: &followRepoStub{},
		ShopUpdateProducer: producer, Redis: redisCli,
	})
	if err := logic.LikeBlog(context.Background(), 51, 12); err != nil {
		t.Fatalf("LikeBlog() error = %v", err)
	}
	if blogRepo.likes != 1 {
		t.Fatalf("likes=%d want=1", blogRepo.likes)
	}
	if len(producer.shopIDs) != 1 || producer.shopIDs[0] != 29 {
		t.Fatalf("vector refresh shops=%v want=[29]", producer.shopIDs)
	}
}
