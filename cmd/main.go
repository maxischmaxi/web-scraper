package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/maxischmaxi/web-scraper/internal/config"
	"github.com/maxischmaxi/web-scraper/internal/database"
	"github.com/maxischmaxi/web-scraper/internal/scraper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func main() {
	cfg := config.Load()

	mongodbCtx, cancelMongodb := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelMongodb()

	mongoDB, err := database.NewMongoDB(mongodbCtx, cfg.MongoURI, cfg.DatabaseName)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}

	defer mongoDB.Close(mongodbCtx)

	collections := database.NewCollections(mongoDB.Database())

	config := &scraper.ScraperConfig{
		Language: "en-US",
	}

	pages, err := collections.Pages.Find(context.Background(), bson.M{})
	if err != nil {
		log.Fatalf("failed to find pages: %v", err)
	}
	defer pages.Close(context.Background())
	var pageList []scraper.Page
	if err := pages.All(context.Background(), &pageList); err != nil {
		log.Fatalf("failed to decode pages: %v", err)
	}
	log.Printf("Found %d pages in the database", len(pageList))

	existingURLs := []string{}
	for _, page := range pageList {
		existingURLs = append(existingURLs, page.URL)
	}

	scraper := scraper.NewScraper(collections, config, existingURLs)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("lang", "de-DE"),
		chromedp.Flag("disable-gpu", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	url := "https://www.reddit.com/"

	err = scraper.ScrapeRecursive(ctx, url)
	if err != nil {
		log.Fatalf("failed to scrape: %v", err)
	}

	log.Println("Scraping completed successfully")
}
