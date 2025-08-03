package scraper

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/maxischmaxi/web-scraper/internal/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Page struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	URL       string        `bson:"url"`
	Timestamp string        `bson:"timestamp"`
	Title     string        `bson:"title"`
	HTML      string        `bson:"html"`
	Content   string        `bson:"content"`
	Links     []string      `bson:"links"`
	Images    []string      `bson:"images"`
	Language  string        `bson:"language"`
}

type Scraper struct {
	collections *database.Collections
	config      *ScraperConfig
	scrapedURLs []string
}

type ScraperConfig struct {
	Language string
}

func NewScraper(collections *database.Collections, config *ScraperConfig, scrapedURLs []string) *Scraper {
	return &Scraper{
		collections: collections,
		config:      config,
		scrapedURLs: scrapedURLs,
	}
}

func (s *Scraper) shouldScrapeURL(url string) bool {
	if !strings.HasPrefix(url, "http") {
		return false
	}

	if slices.Contains(s.scrapedURLs, url) {
		return false
	}

	if strings.Contains(url, ":") {
		return false
	}
	return true
}

func getDocumentContent(doc *goquery.Document) string {
	content := doc.Find("body").Text()
	content = strings.TrimSpace(content)
	re := regexp.MustCompile(`\n+`)
	content = re.ReplaceAllString(content, "\n")
	re = regexp.MustCompile(`[ \t]+`)
	content = re.ReplaceAllString(content, " ")
	return content
}

func getDocumentLinks(doc *goquery.Document, base string) (*[]string, error) {
	var links []string

	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")

		if !exists || strings.TrimSpace(href) == "" {
			return
		}

		relURL, err := url.Parse(href)
		if err != nil {
			return
		}

		if relURL.Fragment != "" {
			// Skip links with fragments
			return
		}

		fullURL := baseURL.ResolveReference(relURL)
		links = append(links, fullURL.String())
	})

	return &links, nil
}

func getDocumentImages(doc *goquery.Document, base string) (*[]string, error) {
	var images []string

	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || strings.TrimSpace(src) == "" {
			return
		}

		relURL, err := url.Parse(src)
		if err != nil {
			return
		}

		fullURL := baseURL.ResolveReference(relURL)
		images = append(images, fullURL.String())
	})

	return &images, nil
}

func (s *Scraper) ScrapeRecursive(ctx context.Context, url string) error {
	page, err := s.ScrapePage(ctx, url)
	if err != nil {
		return err
	}

	if page == nil {
		log.Printf("Page already scraped: %s", url)
		return nil
	}

	links := page.Links

	for _, link := range links {
		if !s.shouldScrapeURL(link) {
			continue
		}

		err := s.ScrapeRecursive(ctx, link)
		if err != nil {
			log.Println(err)
		}
		log.Printf("Scraped page: %s, content length: %d\n", url, len(page.Content))
	}

	return nil
}

func (s *Scraper) ScrapePage(ctx context.Context, url string) (*Page, error) {
	fmt.Println("Scraping URL:", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch URL %s: status code %d", url, resp.StatusCode)
	}

	var pageHTML string

	err = chromedp.Run(ctx,
		chromedp.Sleep(time.Duration(0.5*float64(time.Second))),
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.OuterHTML(`html`, &pageHTML),
	)

	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}

	links, err := getDocumentLinks(doc, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get links from document: %w", err)
	}

	images, err := getDocumentImages(doc, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get images from document: %w", err)
	}

	page := Page{
		ID:        bson.NewObjectID(),
		URL:       url,
		Timestamp: time.Now().Format(time.RFC3339),
		Title:     doc.Find("title").Text(),
		HTML:      pageHTML,
		Content:   getDocumentContent(doc),
		Links:     *links,
		Images:    *images,
		Language:  doc.Find("html").AttrOr("lang", "unknown"),
	}

	res, err := s.collections.Pages.InsertOne(context.Background(), page)
	if err != nil {
		log.Fatalf("failed to insert page into MongoDB: %v", err)
	}

	if res.InsertedID == nil {
		return nil, fmt.Errorf("failed to insert page into MongoDB")
	}

	s.scrapedURLs = append(s.scrapedURLs, url)

	return &page, nil
}
