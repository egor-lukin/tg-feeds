package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/feeds"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const MAX_RSS_POSTS_COUNT = 20

type Channel struct {
	Name        string
	Title       string
	LastId      int
	Link        string
	Description string
}

type Post struct {
	Header    string
	Content   string
	Link      string
	CreatedAt time.Time
}

type DbChannel struct {
	Id          int
	Name        string
	Title       string
	LastId      int
	Link        string
	Description string
}

type DbPost struct {
	Id        int
	Header    string
	Content   string
	Link      string
	CreatedAt time.Time

	ChannelId int
}

type Feed struct {
	Channel Channel
	Posts   []Post
}

type Cache interface {
	GetChannel(name string) (DbChannel, error)
	SaveChannel(channel Channel) (DbChannel, error)
	UpdateLastPostId(channelId int, lastPostId int) error

	GetPosts(channelId int, count int) ([]DbPost, error)
	SavePosts(channelId int, posts []Post) ([]DbPost, error)
}

func main() {
	var dbPath, port string
	flag.StringVar(&dbPath, "dbpath", "file:./tg-feeds.db?cache=shared&mode=rwc", "path to the SQLite database file")
	flag.StringVar(&port, "port", "4567", "GIN server port")

	flag.Parse()

	db, err := initDB(dbPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	cache := &SqliteCache{db: db}
	fetcher := &TelegramWebFetcher{}

	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	r.GET("/:channel", func(c *gin.Context) {
		channelName := c.Param("channel")
		feed, err := prepareFeed(channelName, cache, fetcher)
		if err != nil {
			fmt.Println(err)
			return
		}

		rss, _ := feed.ToRss()
		c.Data(http.StatusOK, "application/xml", []byte(rss))
	})

	r.Run(":" + port)
}

type SqliteCache struct {
	db *sql.DB
}

func (cache *SqliteCache) GetChannel(name string) (DbChannel, error) {
	var channel DbChannel
	query := "SELECT id, name, title, lastId, link, description FROM channels WHERE name = ?"
	err := cache.db.QueryRow(query, name).Scan(&channel.Id, &channel.Name, &channel.Title, &channel.LastId, &channel.Link, &channel.Description)
	return channel, err
}

func (cache *SqliteCache) SaveChannel(channel Channel) (DbChannel, error) {
	query := `
		INSERT INTO channels (name, title, lastId, link, description)
		VALUES (?, ?, ?, ?, ?)`
	res, err := cache.db.Exec(query, channel.Name, channel.Title, channel.LastId, channel.Link, channel.Description)

	lastInsertId, err := res.LastInsertId()
	if err != nil {
		return DbChannel{}, err
	}

	dbChannel := DbChannel{
		Id:          int(lastInsertId),
		Name:        channel.Name,
		Title:       channel.Title,
		LastId:      channel.LastId,
		Link:        channel.Link,
		Description: channel.Description,
	}

	return dbChannel, err
}

func (cache *SqliteCache) UpdateLastPostId(channelId int, lastPostId int) error {
	query := "UPDATE channels SET lastId = ? WHERE id = ?"
	_, err := cache.db.Exec(query, lastPostId, channelId)
	return err
}

func (cache *SqliteCache) GetPosts(channelId int, count int) ([]DbPost, error) {
	posts := []DbPost{}
	query := "SELECT id, header, content, link, createdAt FROM posts WHERE channelId = ? ORDER BY createdAt DESC LIMIT ?"
	rows, err := cache.db.Query(query, channelId, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var post DbPost
		err := rows.Scan(&post.Id, &post.Header, &post.Content, &post.Link, &post.CreatedAt)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func (cache *SqliteCache) SavePosts(channelId int, posts []Post) ([]DbPost, error) {
	tx, err := cache.db.Begin()
	var savedPosts []DbPost

	if err != nil {
		return savedPosts, err
	}

	stmt, err := tx.Prepare("INSERT INTO posts (header, content, link, createdAt, channelId) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return savedPosts, err
	}
	defer stmt.Close()

	for _, post := range posts {
		res, err := stmt.Exec(post.Header, post.Content, post.Link, post.CreatedAt, channelId)
		if err != nil {
			tx.Rollback()
			return savedPosts, err
		}

		insertedId, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return savedPosts, err
		}

		savedPost := DbPost{Id: int(insertedId), Header: post.Header, Content: post.Content, Link: post.Link, CreatedAt: post.CreatedAt, ChannelId: channelId}
		savedPosts = append(savedPosts, savedPost)
	}

	if err := tx.Commit(); err != nil {
		return savedPosts, err
	}

	return savedPosts, nil
}

type Fetcher interface {
	FetchChannel(channelName string) (Channel, error)
	FetchPost(channelName string, id int) (Post, error)
}

type TelegramWebFetcher struct{}

func (fetcher *TelegramWebFetcher) FetchChannel(channelName string) (Channel, error) {
	url := tgChannelFeedUrl(channelName)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return Channel{}, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)

	var description, dataPost, title string
	var split []string
	var currentId int
	lastId := -1

	doc.Find(".tgme_widget_message").Each(func(i int, s *goquery.Selection) {
		dataPost, _ = s.Attr("data-post")
		split = strings.Split(dataPost, "/")
		currentId, _ = strconv.Atoi(split[1])

		if lastId == -1 || currentId > lastId {
			lastId = currentId
		}
	})

	if lastId == -1 {
		return Channel{}, errors.New("Can't parse channel page")
	}

	doc.Find(".tgme_channel_info_header_title").Each(func(i int, s *goquery.Selection) {
		title = s.Find("span").Text()
	})

	doc.Find(".tgme_channel_info_description").Each(func(i int, s *goquery.Selection) {
		description = s.Text()
	})

	channel := Channel{Name: channelName, Title: title, LastId: lastId, Link: url, Description: description}
	return channel, nil
}

func (fetcher *TelegramWebFetcher) FetchPost(channelName string, id int) (Post, error) {
	url := tgChannelPostUrl(channelName, id)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return Post{}, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return Post{}, err
	}

	error_message := ""
	doc.Find(".tgme_widget_message_error").Each(func(i int, s *goquery.Selection) {
		error_message = s.Text()
	})

	if error_message != "" {
		return Post{}, errors.New(error_message)
	}

	var content string
	doc.Find(".tgme_widget_message_text.js-message_text").Each(func(i int, s *goquery.Selection) {
		content = s.Text()
	})

	var createdAt time.Time
	layout := "2006-01-02T15:04:05Z07:00"

	doc.Find(".tgme_widget_message_date").Each(func(i int, s *goquery.Selection) {
		datetime, _ := s.Find("time").Attr("datetime")
		createdAt, err = time.Parse(layout, datetime)
		if err != nil {
			fmt.Printf("err %s\n", err)
		}
	})

	var headerContent string
	if len(content) > 100 {
		headerContent = strings.Trim(content[0:100], " ") + "..."
	}

	content = content + "\n\n" + "<a href=\"" + url + "\">[link]</a>"

	return Post{Header: headerContent, Content: content, Link: url, CreatedAt: createdAt}, nil
}

func initDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	createChannelsTable := `
        CREATE TABLE IF NOT EXISTS channels (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT UNIQUE NOT NULL,
            title TEXT NOT NULL,
            lastId INTEGER NOT NULL,
            link TEXT NOT NULL,
            description TEXT
        );

		CREATE UNIQUE INDEX IF NOT EXISTS channel_name ON channels(name);`

	createPostsTable := `
        CREATE TABLE IF NOT EXISTS posts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            channelId INTEGER NOT NULL,
            header TEXT NOT NULL,
            content TEXT NOT NULL,
            link TEXT NOT NULL,
            createdAt DATETIME NOT NULL,
            FOREIGN KEY(channelId) REFERENCES channels(id)
        );`

	_, err = db.Exec(createChannelsTable)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(createPostsTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func prepareFeed(channelName string, cache Cache, fetcher Fetcher) (*feeds.Feed, error) {
	channel, err := fetcher.FetchChannel(channelName)
	feed := &feeds.Feed{}

	if err == nil {
		dbCachedChannel, err := cache.GetChannel(channelName)

		if err != nil {
			newChannel := Channel{Name: channel.Name, Title: channel.Title, LastId: 0, Link: channel.Link, Description: channel.Description}
			dbCachedChannel, _ = cache.SaveChannel(newChannel)
		}

		var dbPosts []DbPost
		var posts []Post

		if dbCachedChannel.LastId == channel.LastId {
			dbPosts, err = cache.GetPosts(dbCachedChannel.Id, MAX_RSS_POSTS_COUNT)
			if err == nil {
				feed = generateFeed(dbCachedChannel, dbPosts)

				return feed, nil
			} else {
				fmt.Printf("Problem with cached posts: %s\n", err)

				return feed, err
			}
		} else {
			var postId = channel.LastId

			for postId > 0 && len(posts) < MAX_RSS_POSTS_COUNT {
				fmt.Printf("[%s] Download Post: %d\n", channelName, postId)

				post, err := fetcher.FetchPost(channel.Name, postId)
				postId--

				if err != nil {
					fmt.Printf("Error: %s\n", err)
					continue
				}

				if len(posts) > 0 && post.CreatedAt == posts[len(posts)-1].CreatedAt {
					fmt.Printf("Duplicated post")
					continue
				}

				if postId == dbCachedChannel.LastId {
					break
				}

				posts = append(posts, post)
			}

			cache.UpdateLastPostId(dbCachedChannel.Id, channel.LastId)
			newDbPosts, err := cache.SavePosts(dbCachedChannel.Id, posts)
			if err != nil {
				fmt.Printf("Can't save posts -%s\n", err)
				return feed, nil
			}

			feed := generateFeed(dbCachedChannel, newDbPosts)

			return feed, nil
		}
	} else {
		fmt.Printf("Fetch telegram channel failed: %s\n", err)

		return feed, err
	}
}

func generateFeed(channel DbChannel, posts []DbPost) *feeds.Feed {
	feed := &feeds.Feed{
		Title:       channel.Name,
		Link:        &feeds.Link{Href: channel.Link},
		Description: channel.Description,
	}

	var item *feeds.Item
	var items []*feeds.Item
	for _, post := range posts {
		item = &feeds.Item{
			Title:       post.Header,
			Link:        &feeds.Link{Href: post.Link},
			Description: post.Content,
			Created:     post.CreatedAt,
		}

		items = append(items, item)
	}

	feed.Items = items

	return feed
}

func tgChannelPostUrl(channelName string, id int) string {
	url := "https://t.me/" + channelName + "/" + strconv.Itoa(id) + "?embed=1&mode=tme"
	return url
}

func tgChannelFeedUrl(channelName string) string {
	url := "https://t.me/s/" + channelName
	return url
}
