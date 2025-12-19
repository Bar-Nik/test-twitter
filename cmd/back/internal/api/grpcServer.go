package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	pb "twitter/api/proto/v1"
	"twitter/cmd/back/internal/app"

	"github.com/gofrs/uuid/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	MessageQueue = "message"
)

type Mess struct {
	Message string `json:"message"`
}

type Repository interface {
	CreateTweetToDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error)
	GetTweetByIDFromDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error)
	GetUserTweetsFromDB(ctx context.Context, userId uuid.UUID) ([]app.Tweet, error)
	UpdateTweetToDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error)
	DeleteTweetFromDB(ctx context.Context, tweet app.Tweet) error
	GetSubscribersTweetsFromDB(ctx context.Context, userIds []uuid.UUID) ([]app.Tweet, error)
}

type CacheTweets interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	GetDelete(ctx context.Context, key string) (string, error)
}
type CacheUserTweet interface {
	AddToRight(ctx context.Context, key string, items ...string) error
	GetList(ctx context.Context, key string) ([]string, error)
	RemoveElements(ctx context.Context, key string, value string) (int64, error)
}
type Producer interface {
	PublishJSON(ctx context.Context, routingKey string, message interface{}) error
}

type GrpcServer struct {
	Database          Repository
	JwtSecret         string
	CacheDBTweets     CacheTweets
	CacheDBUserTweets CacheUserTweet
	Producer          Producer
}

// const authScheme = "Bearer"

func (s GrpcServer) CreateTweet(ctx context.Context, request *pb.CreateTweetRequest) (*pb.CreateTweetResponse, error) {

	userId, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	newTweet := app.Tweet{
		Text:   request.Text,
		UserId: uuid.FromStringOrNil(userId),
	}
	tweet, err := s.Database.CreateTweetToDB(ctx, newTweet)
	if err != nil {

		return nil, fmt.Errorf("CreateTweetToDB: %w", err)
	}

	tweetJSON, err := json.Marshal(tweet)
	if err != nil {
		fmt.Println("Ошибка сериализации:", err)
	}

	err = s.CacheDBTweets.Set(ctx, tweet.Id.String(), tweetJSON, 10*time.Minute)
	if err != nil {
		fmt.Println("Ошибка SET:", err)
	} else {
		fmt.Println("SET операция выполнена успешно")
	}

	// используется для GetUserTweets
	err = s.CacheDBUserTweets.AddToRight(ctx, tweet.UserId.String(), string(tweetJSON))
	if err != nil {
		fmt.Println("Ошибка AddToRight:", err)
	} else {
		fmt.Println("AddToRight операция выполнена успешно (CreateTweet)")
	}

	// отправить в очередь
	message := Mess{Message: "Create Tweet"}
	err = s.Producer.PublishJSON(ctx, MessageQueue, message)
	if err != nil {
		fmt.Println("Rabbit error Create:", err)
	}

	return &pb.CreateTweetResponse{
		Tweet: &pb.Tweet{
			Id:        tweet.Id.String(),
			Text:      tweet.Text,
			CreatedAt: timestamppb.New(tweet.CreatedAt),
			UpdatedAt: timestamppb.New(tweet.UpdatedAt),
			UserId:    tweet.UserId.String(),
		},
	}, nil
}

func (s GrpcServer) GetTweetByID(ctx context.Context, request *pb.GetTweetByIDRequest) (*pb.GetTweetByIDResponse, error) {

	tweet := app.Tweet{
		Id: uuid.FromStringOrNil(request.Id),
	}

	tweetRedis, err := s.CacheDBTweets.Get(ctx, request.Id)
	if err != nil {
		fmt.Println("Нет в редис", err)
		tweet, err = s.Database.GetTweetByIDFromDB(ctx, tweet)
		if err != nil {
			return nil, fmt.Errorf("GetTweetByIDFromDB: %w", err)
		}
	} else {
		err := json.Unmarshal([]byte(tweetRedis), &tweet)
		if err != nil {
			fmt.Println("Ошибка десериализации GetTweetByID:", err)
		}
	}

	return &pb.GetTweetByIDResponse{Tweet: &pb.Tweet{
		Id:        tweet.Id.String(),
		Text:      tweet.Text,
		CreatedAt: timestamppb.New(tweet.CreatedAt),
		UpdatedAt: timestamppb.New(tweet.UpdatedAt),
		UserId:    tweet.UserId.String(),
	},
	}, nil
}

func (s GrpcServer) GetUserTweets(ctx context.Context, request *pb.GetUserTweetsRequest) (*pb.GetUserTweetsResponse, error) {

	userId, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var pbTweets []*pb.Tweet

	tweetsRedis, err := s.CacheDBUserTweets.GetList(ctx, userId)
	if err != nil {
		fmt.Println("Нет в редис", err)
		tweets, err := s.Database.GetUserTweetsFromDB(ctx, uuid.FromStringOrNil(userId))
		if err != nil {
			return nil, fmt.Errorf("GetUserTweetsFromDB: %w", err)
		}
		pbTweets = make([]*pb.Tweet, len(tweets))
		for i := range tweets {
			pbTweets[i] = toTweet(tweets[i])
		}
	} else {
		pbTweets = make([]*pb.Tweet, len(tweetsRedis))
		for i, t := range tweetsRedis {
			var tweet app.Tweet
			err := json.Unmarshal([]byte(t), &tweet)
			pbTweets[i] = toTweet(tweet)
			if err != nil {
				fmt.Println("Ошибка десериализации GetUserTweets:", err)
			}
		}

	}
	return &pb.GetUserTweetsResponse{
		Tweets: pbTweets,
	}, nil
}

func (s GrpcServer) UpdateTweet(ctx context.Context, request *pb.UpdateTweetRequest) (*pb.UpdateTweetResponse, error) {

	userId, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	newTweet := app.Tweet{
		Id:     uuid.FromStringOrNil(request.Id),
		Text:   request.Text,
		UserId: uuid.FromStringOrNil(userId),
	}
	tweet, err := s.Database.UpdateTweetToDB(ctx, newTweet)
	if err != nil {

		return nil, fmt.Errorf("UpdateTweetToDB: %w", err)
	}

	tweetJSON, err := json.Marshal(tweet)
	if err != nil {
		fmt.Println("Ошибка сериализации:", err)
	}

	err = s.CacheDBTweets.Set(ctx, tweet.Id.String(), tweetJSON, 10*time.Minute)
	if err != nil {
		fmt.Println("Ошибка SET:", err)
	} else {
		fmt.Println("SET операция выполнена успешно")
	}

	// отправить в очередь
	message := Mess{Message: "Update Tweet"}
	err = s.Producer.PublishJSON(ctx, MessageQueue, message)
	if err != nil {
		fmt.Println("Rabbit error Update:", err)
	}

	return &pb.UpdateTweetResponse{Tweet: &pb.Tweet{
		Id:        tweet.Id.String(),
		Text:      tweet.Text,
		CreatedAt: timestamppb.New(tweet.CreatedAt),
		UpdatedAt: timestamppb.New(tweet.UpdatedAt),
		UserId:    tweet.UserId.String(),
	},
	}, nil
}

func (s GrpcServer) DeleteTweet(ctx context.Context, request *pb.DeleteTweetRequest) (*pb.DeleteTweetResponse, error) {

	userId, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	tweet := app.Tweet{
		Id:     uuid.FromStringOrNil(request.Id),
		UserId: uuid.FromStringOrNil(userId),
	}

	err = s.Database.DeleteTweetFromDB(ctx, tweet)
	if err != nil {
		return nil, fmt.Errorf("DeleteTweet: %w", err)
	}

	tweetJSON, err := s.CacheDBTweets.GetDelete(ctx, tweet.Id.String())
	if err != nil {
		fmt.Println("Ошибка CacheDBTweets.GetDelete:", err)
	} else {
		fmt.Println("CacheDBTweets.GetDelete операция выполнена успешно")
	}

	_, err = s.CacheDBUserTweets.RemoveElements(ctx, tweet.UserId.String(), tweetJSON)
	if err != nil {
		fmt.Println("Ошибка RemoveElements:", err)
	}

	// отправить в очередь
	message := Mess{Message: "delete"} //id tweet отправить
	err = s.Producer.PublishJSON(ctx, MessageQueue, message)
	if err != nil {
		fmt.Println("Rabbit error Delete:", err)
	}

	return &pb.DeleteTweetResponse{}, nil
}

func (s GrpcServer) GetSubscribersTweets(ctx context.Context, request *pb.GetSubscribersTweetsRequest) (*pb.GetSubscribersTweetsResponse, error) {
	listSubscribers := request.UserIds
	listSubscribersUuids := make([]uuid.UUID, len(listSubscribers))
	for i := range listSubscribers {
		listSubscribersUuids[i] = uuid.FromStringOrNil(listSubscribers[i])
	}
	tweets, err := s.Database.GetSubscribersTweetsFromDB(ctx, listSubscribersUuids)
	if err != nil {
		return nil, fmt.Errorf("GetSubscribersTweetsFromDB: %w", err)
	}
	pbTweets := make([]*pb.Tweet, len(tweets))
	for i := range tweets {
		pbTweets[i] = toTweet(tweets[i])
	}

	return &pb.GetSubscribersTweetsResponse{Tweets: pbTweets}, nil
}

func toTweet(t app.Tweet) *pb.Tweet {
	return &pb.Tweet{
		Id:        t.Id.String(),
		Text:      t.Text,
		CreatedAt: timestamppb.New(t.CreatedAt),
		UpdatedAt: timestamppb.New(t.UpdatedAt),
		UserId:    t.UserId.String(),
	}
}
