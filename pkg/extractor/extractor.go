package extractor

import (
	"context"
	"fmt"
	"time"

	"github.com/byteowlz/scrpr/internal/browser"
	"github.com/byteowlz/scrpr/internal/config"
	"github.com/byteowlz/scrpr/internal/fetcher"
	"github.com/byteowlz/scrpr/internal/processor"
)

type Extractor struct {
	config    *config.Config
	fetcher   *fetcher.ContentFetcher
	processor *processor.ContentProcessor
	cookies   *browser.CookieExtractor
}

type ExtractOptions struct {
	Format          string
	IncludeMetadata bool
	UseJS           *bool // nil = auto, true = force, false = disable
	Timeout         time.Duration
}

type ExtractResult struct {
	URL            string
	Title          string
	Content        string
	UsedJavaScript bool
	ProcessingTime time.Duration
	ContentLength  int
	Metadata       map[string]string
}

func New(cfg *config.Config) *Extractor {
	return &Extractor{
		config:    cfg,
		fetcher:   fetcher.NewContentFetcher(),
		processor: processor.NewContentProcessor(),
		cookies:   browser.NewCookieExtractor(browser.BrowserType(cfg.Browser.Default), cfg.Browser.Paths),
	}
}

func (e *Extractor) Extract(ctx context.Context, url string, opts ExtractOptions) (*ExtractResult, error) {
	start := time.Now()

	// Extract cookies for the URL
	cookies, err := e.cookies.ExtractCookies(url)
	if err != nil {
		// Cookie extraction failure is not fatal, log and continue
		cookies = nil
	}

	// Determine fetch mode
	fetchMode := fetcher.FetchModeAuto
	if opts.UseJS != nil {
		if *opts.UseJS {
			fetchMode = fetcher.FetchModeJS
		} else {
			fetchMode = fetcher.FetchModeStatic
		}
	}

	// Set up fetch options
	fetchOpts := fetcher.FetchOptions{
		Mode:            fetchMode,
		Timeout:         opts.Timeout,
		UserAgent:       e.config.Network.UserAgent,
		Cookies:         cookies,
		SkipBanners:     e.config.Extraction.SkipCookieBanners,
		BannerTimeout:   time.Duration(e.config.Extraction.BannerTimeout) * time.Second,
		WaitForSelector: e.config.Extraction.WaitForSelector,
	}

	// Fetch content
	fetchResult, err := e.fetcher.Fetch(ctx, url, fetchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	// Set up processing options
	processOpts := processor.ProcessOptions{
		RemoveAds:        e.config.Extraction.RemoveAds,
		CleanHTML:        e.config.Extraction.CleanHTML,
		MinContentLength: e.config.Extraction.MinContentLength,
		IncludeMetadata:  opts.IncludeMetadata,
		MetadataFields:   e.config.Output.MetadataFields,
	}

	// Process content
	processed, err := e.processor.Process(fetchResult.HTML, url, processOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to process content: %w", err)
	}

	// Format output
	var content string
	switch opts.Format {
	case "markdown":
		content = e.processor.ToMarkdown(processed, opts.IncludeMetadata, e.config.Output.PreserveLinks)
	case "text":
		content = e.processor.ToText(processed, e.config.Output.LineWidth)
	default:
		content = processed.TextContent
	}

	processingTime := time.Since(start)

	return &ExtractResult{
		URL:            url,
		Title:          processed.Title,
		Content:        content,
		UsedJavaScript: fetchResult.UsedJS,
		ProcessingTime: processingTime,
		ContentLength:  len(content),
		Metadata:       processed.Metadata,
	}, nil
}
