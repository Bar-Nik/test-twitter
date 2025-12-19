package repo

import (
	"context"
	"database/sql"
	"fmt"
	"twitter/cmd/back/internal/app"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(rawDB *sql.DB) *Repository {
	return &Repository{db: rawDB}
}

func (d Repository) CreateTweetToDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error) {
	query := `insert into tweets (text, user_id) values ($1, $2) returning *`
	err := d.db.QueryRowContext(ctx, query, tweet.Text, tweet.UserId).Scan(&tweet.Id, &tweet.Text,
		&tweet.CreatedAt, &tweet.UpdatedAt, &tweet.UserId)
	if err != nil {
		return app.Tweet{}, err
	}
	return tweet, nil
}

func (d Repository) GetTweetByIDFromDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error) {
	query := `select * from tweets where id = $1`

	err := d.db.QueryRowContext(ctx, query, tweet.Id).Scan(&tweet.Id, &tweet.Text,
		&tweet.CreatedAt, &tweet.UpdatedAt, &tweet.UserId)
	if err != nil {
		return app.Tweet{}, err
	}
	return tweet, nil
}

func (d Repository) GetUserTweetsFromDB(ctx context.Context, userId uuid.UUID) ([]app.Tweet, error) {
	query := `select * from tweets where user_id = $1`
	row, err := d.db.QueryContext(ctx, query, userId)
	if err != nil {
		return []app.Tweet{}, err
	}
	var tweets []app.Tweet
	for row.Next() {
		var tweet app.Tweet
		row.Scan(&tweet.Id, &tweet.Text,
			&tweet.CreatedAt, &tweet.UpdatedAt, &tweet.UserId)
		tweets = append(tweets, tweet)
	}
	return tweets, nil
}

func (d Repository) UpdateTweetToDB(ctx context.Context, tweet app.Tweet) (app.Tweet, error) {
	fmt.Println("----", tweet.Id, tweet.UserId, "-", tweet.Text)
	query := `update tweets
	set
	text = $1
	where id = $2 and user_id = $3
	returning *
	`

	err := d.db.QueryRowContext(ctx, query, tweet.Text, tweet.Id, tweet.UserId).Scan(&tweet.Id, &tweet.Text,
		&tweet.CreatedAt, &tweet.UpdatedAt, &tweet.UserId)
	if err != nil {
		return app.Tweet{}, err
	}
	return tweet, nil
}

func (d Repository) DeleteTweetFromDB(ctx context.Context, tweet app.Tweet) error {
	query := `delete from tweets where id = $1 and user_id = $2`
	_, err := d.db.ExecContext(ctx, query, tweet.Id, tweet.UserId)
	if err != nil {
		return err
	}
	return nil
}

func (d Repository) GetSubscribersTweetsFromDB(ctx context.Context, userIds []uuid.UUID) ([]app.Tweet, error) {
	query := `select * from tweets where user_id =ANY ($1)`
	row, err := d.db.QueryContext(ctx, query, pq.Array(userIds))
	if err != nil {
		return []app.Tweet{}, err
	}
	defer row.Close()
	var tweets []app.Tweet
	for row.Next() {
		var tweet app.Tweet
		row.Scan(&tweet.Id, &tweet.Text,
			&tweet.CreatedAt, &tweet.UpdatedAt, &tweet.UserId)
		tweets = append(tweets, tweet)
	}
	return tweets, nil
}
