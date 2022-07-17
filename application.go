package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

type User struct {
	Id       string `json:"id"`
	Name     string `json:"name`
	Username string `json:username`
}

type Tweet struct {
	Id        string `json:"id"`
	Authorid  string `json:"authorid"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type TweetSmall struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

type ResponseMeta struct {
	Result_count int    `json:"result_count"`
	Newest_id    string `json:"newest_id"`
	Oldest_id    string `json:"oldest_id"`
	Next_token   string `json:"next_token"`
}

type Response struct {
	Data []TweetSmall `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

func main() {
	e := echo.New()
	client := resty.New()
	db, err := sql.Open("postgres", os.Getenv("DATABASEURL"))

	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	e.GET("/", func(c echo.Context) error {
		// sql := "INSERT INTO users(id, name, username) VALUES('1894180640', 'B0aty', 'B0aty')"

		// if _, err := db.Exec(sql); err != nil {
		// 	c.String(http.StatusInternalServerError, err.Error())
		// }

		// if _, err := db.Exec("INSERT INTO tweets(id, authorid, text, timestamp) VALUES('1546897437318586371', '1894180640', 'These new images from the @NASAWebb Space Telescope are mind-blowing. Congratulations to everybody who made this possible—I’m excited to see more! https://t.co/dRDlsav92L', now())"); err != nil {
		// 	c.String(http.StatusInternalServerError, err.Error())
		// }

		return c.String(http.StatusOK, "Running on EBS!")
	})

	// Query serverless fn for user, insert into DB
	e.GET("/user", func(c echo.Context) error {
		username := c.QueryParam("username")
		if username == "" {
			return c.String(http.StatusBadRequest, "Missing params: username")
		}
		var user User
		resp, err := client.R().SetQueryParam("username", username).SetResult(&User{}).Get("https://orbital-api-eta.vercel.app/api/userbyusername")
		if err != nil {
			log.Fatal(err)
			return c.String(http.StatusBadRequest, err.Error())
		}
		if resp.StatusCode() == 200 {
			user = *resp.Result().(*User)
			// Insert into DB
			_, err := db.Exec(`INSERT INTO users(id, name, username) VALUES($1, $2, $3)`, user.Id, user.Name, user.Username)
			if err != nil {
				fmt.Println(err.Error())
				//return c.String(http.StatusInternalServerError, err.Error())
			}
			return c.JSON(http.StatusOK, (*resp.Result().(*User)))
		} else {
			return c.String(resp.StatusCode(), string(resp.Body()))
		}
	})

	e.GET("/tweet", func(c echo.Context) error {
		id := c.QueryParam("id")
		var tweet Tweet
		row := db.QueryRow(`SELECT * FROM tweets WHERE authorid=$1`, id)
		err := row.Scan(&tweet.Id, &tweet.Authorid, &tweet.Text, &tweet.Timestamp)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, tweet)

	})

	/*
		Check DB
			if exists && timestamp within 12 hours: retrieve
			if exists && timestamp older: delete all -> hydrate()
			if does not exist -> hydrate()

			hydrate() : query serverless fn for timeline, store in db
	*/
	e.GET("/tweets", func(c echo.Context) error {
		id := c.QueryParam("id")
		if id == "" {
			return c.String(http.StatusBadRequest, "Missing params: id")
		}
		// Check Postgres
		rows, err := db.Query(`SELECT * FROM tweets WHERE authorid=$1`, id)
		defer rows.Close()
		if err != nil {
			return err
		}
		// Store in tweets slice
		var tweets []Tweet
		for rows.Next() {

			var tweet Tweet
			if err := rows.Scan(&tweet.Id, &tweet.Authorid, &tweet.Text, &tweet.Timestamp); err != nil {
				return err
			}
			tweets = append(tweets, tweet)
		}

		//fmt.Print(tweets)

		// Check length of tweets
		if len(tweets) == 0 {

			fmt.Println("No tweets exist, getting new tweets")
			smallTweets, err := hydrate(db, client, id)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, smallTweets)

		} else {
			// Some exist - check timestamp.
			tweetTime, error := time.Parse("2006-01-02T15:04:05.000000Z", tweets[0].Timestamp)
			if error != nil {
				fmt.Println(error)
			}
			today := time.Now()
			// Tweets older than 12 hours - delete and hydrate
			if today.After(tweetTime.Add(12 * time.Hour)) {
				_, err := db.Exec("DELETE FROM tweets WHERE authorid=$1", id)
				if err != nil {
					fmt.Println(error)
				}
				// hydrate
				smallTweets, err := hydrate(db, client, id)
				if err != nil {
					return err
				}
				fmt.Println("Tweets already exist but older than 12 hours, returning new tweets")
				return c.JSON(http.StatusOK, smallTweets)
			} else {
				// Just return cached tweets
				fmt.Println("Tweets already exist and newer than 12 hours, returning cached tweets")
				return c.JSON(http.StatusOK, tweets)
			}

		}
	})
	e.Logger.Fatal(e.Start(":5000"))
}

func hydrate(db *sql.DB, client *resty.Client, id string) ([]TweetSmall, error) {
	fmt.Println("hydrating...")
	// Query serverless fn for timeline
	resp, err := client.R().SetQueryParams(map[string]string{
		"id":          id,
		"exclude":     "retweets,replies",
		"max_results": "50",
	}).SetResult(&Response{}).Get("https://orbital-api-eta.vercel.app/api/usertimeline")

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	if resp.StatusCode() == 200 {
		// Store
		var tweets []TweetSmall
		query := `INSERT INTO tweets(id, authorid, text, timestamp) VALUES`
		tweets = (*resp.Result().(*Response)).Data
		// Build query
		for i, tweet := range tweets {
			if i == len(tweets)-1 {
				query += fmt.Sprintf(`('%s', '%s', '%s', now());`, tweet.Id, id, strings.Replace(tweet.Text, "'", "''", -1))
			} else {
				query += fmt.Sprintf(`('%s', '%s', '%s', now()),`, tweet.Id, id, strings.Replace(tweet.Text, "'", "''", -1))
			}
		}
		//fmt.Println(query)

		//Execute db query
		_, err := db.Exec(query)
		if err != nil {
			fmt.Println("Error")
			fmt.Printf(err.Error())
		}
		return tweets, nil
	} else {
		return nil, fmt.Errorf("error with request to api, check params")
	}

}
