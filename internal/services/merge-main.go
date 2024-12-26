package services

import (
	"brancher/internal/utils"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
)

func (s *BranchService) MergeMainToDirectories() error {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to list directories: %w", err)
	}

	for _, file := range files {
		if file.IsDir() && strings.HasPrefix(file.Name(), "repo-") {
			dir := file.Name()
			branchName := strings.Replace(dir, "repo-", "", 1)
			safeBranchName := strings.ReplaceAll(branchName, "_", "/")
			log.Printf("Merging 'main' into branch '%s' in GitHub...", branchName)

			mergeURL := fmt.Sprintf("%s/repos/%s/%s/merges", s.Config.GitHubAPIURL, s.Config.RepoOwner, s.Config.RepoName)
			payload := map[string]string{
				"base":           safeBranchName,
				"head":           "main",
				"commit_message": fmt.Sprintf("Merging 'main' into '%s'", safeBranchName),
			}

			headers := map[string]string{
				"Authorization": fmt.Sprintf("token %s", s.Config.GitHubToken),
				"Accept":        "application/vnd.github.v3+json",
			}

			if _, err := utils.MakeRequest("POST", mergeURL, headers, payload); err != nil {
				log.Printf("Failed to merge 'main' into branch '%s': %v", safeBranchName, err)
			} else {
				log.Printf("Successfully merged 'main' into branch '%s'", safeBranchName)
			}

			mainFilePath := path.Join(dir, "main")
			if err := os.Remove(mainFilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove old binary: %w", err)
			}

			log.Printf("Merging 'main' into local directory: %s", dir)
			mergeCmd := fmt.Sprintf("cd %s && git checkout main && git pull origin main && git checkout . && git merge main", dir)
			if err := utils.ExecuteCommand(mergeCmd); err != nil {
				log.Printf("Local merge failed for directory %s: %v", dir, err)
			} else {
				log.Printf("Successfully merged 'main' locally in directory %s", dir)
			}

			if err := buildMainFile(dir); err != nil {
				log.Printf("Failed to build main file in directory %s: %v", dir, err)
			} else {
				log.Printf("Successfully built main file in directory %s", dir)
			}
		}
	}

	return nil
}

func buildMainFile(directory string) error {
	mainFilePath := path.Join(directory, "main")
	if err := os.Remove(mainFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old binary: %w", err)
	}

	buildCmd := fmt.Sprintf("cd %s && go build -o main main.go", directory)
	if err := utils.ExecuteCommand(buildCmd); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	log.Printf("New binary built at %s", mainFilePath)
	return nil
}
