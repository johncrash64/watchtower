package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"watchtower/internal/config"
	"watchtower/internal/models"
	"watchtower/internal/parse"
	"watchtower/internal/store"
	"watchtower/internal/util"
)

type StudyPaths struct {
	RootDir   string
	SourceDir string
	DataDir   string
	OutputDir string
	DBPath    string
}

type Result struct {
	Study       models.Study
	Article     models.ParsedArticle
	Paths       StudyPaths
	StoredInput string
}

func ResolveStudyPaths(baseDir, weekID string) StudyPaths {
	root := filepath.Join(baseDir, weekID)
	return StudyPaths{
		RootDir:   root,
		SourceDir: filepath.Join(root, "source"),
		DataDir:   filepath.Join(root, "data"),
		OutputDir: filepath.Join(root, "outputs"),
		DBPath:    filepath.Join(root, "data", "study.db"),
	}
}

func Ingest(ctx context.Context, cfg config.Config, weekID, inputPath, docIDOverride string) (Result, error) {
	normWeek, err := util.NormalizeWeekID(weekID)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(inputPath) == "" {
		return Result{}, fmt.Errorf("--input is required")
	}

	paths := ResolveStudyPaths(cfg.StudiesDir, normWeek)
	for _, dir := range []string{paths.RootDir, paths.SourceDir, paths.DataDir, paths.OutputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, err
		}
	}

	article, err := parse.ParseInput(inputPath)
	if err != nil {
		return Result{}, err
	}
	if docIDOverride != "" {
		article.DocID = strings.TrimSpace(docIDOverride)
	}

	storedInput, checksum, sourceType, err := copyInput(paths.SourceDir, inputPath)
	if err != nil {
		return Result{}, err
	}

	db, err := store.Open(paths.DBPath)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()

	study, err := db.EnsureStudy(ctx, normWeek, article.DocID, article.Title, article.DateRange, article.Language)
	if err != nil {
		return Result{}, err
	}
	if err := db.InsertSource(ctx, study.ID, sourceType, storedInput, checksum); err != nil {
		return Result{}, err
	}
	if err := db.ReplaceParagraphs(ctx, study.ID, article.Paragraphs); err != nil {
		return Result{}, err
	}

	return Result{
		Study:       study,
		Article:     article,
		Paths:       paths,
		StoredInput: storedInput,
	}, nil
}

func copyInput(destDir, inputPath string) (storedPath, checksum, sourceType string, err error) {
	in, err := os.Open(inputPath)
	if err != nil {
		return "", "", "", err
	}
	defer in.Close()

	name := filepath.Base(inputPath)
	name = sanitizeFilename(name)
	if name == "" {
		name = "article"
	}
	storedPath = filepath.Join(destDir, name)

	out, err := os.Create(storedPath)
	if err != nil {
		return "", "", "", err
	}
	defer out.Close()

	h := sha256.New()
	mw := io.MultiWriter(out, h)
	if _, err := io.Copy(mw, in); err != nil {
		return "", "", "", err
	}
	checksum = hex.EncodeToString(h.Sum(nil))
	ext := strings.ToLower(filepath.Ext(name))
	sourceType = strings.TrimPrefix(ext, ".")
	if sourceType == "htm" {
		sourceType = "html"
	}
	if sourceType == "" {
		sourceType = "file"
	}
	return storedPath, checksum, sourceType, nil
}

func sanitizeFilename(v string) string {
	v = strings.TrimSpace(v)
	replacer := strings.NewReplacer("/", "-", "\\", "-", "..", "-")
	v = replacer.Replace(v)
	return v
}
