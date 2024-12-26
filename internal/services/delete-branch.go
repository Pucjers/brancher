package services

import (
	"brancher/internal/utils"
	"fmt"
	"log"
	"os"
	"strings"
)

func (s *BranchService) DeleteBranchAndService(branchName string) error {
	branchName = s.Config.BranchPrefix + branchName
	headers := map[string]string{
		"Authorization": fmt.Sprintf("token %s", s.Config.GitHubToken),
		"Accept":        "application/vnd.github.v3+json",
	}
	url := fmt.Sprintf("%s/repos/%s/%s/git/refs/heads/%s", s.Config.GitHubAPIURL, s.Config.RepoOwner, s.Config.RepoName, branchName)
	_, err := utils.MakeRequest("DELETE", url, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", branchName, err)
	}

	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	cmds := []string{
		fmt.Sprintf("systemctl stop %s.service", safeBranchName),
		fmt.Sprintf("systemctl disable %s.service", safeBranchName),
		fmt.Sprintf("rm /etc/systemd/system/%s.service", safeBranchName),
		"systemctl daemon-reload",
	}
	for _, cmd := range cmds {
		if err := utils.ExecuteCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute %s: %w", cmd, err)
		}
	}

	localDir := fmt.Sprintf("repo-%s", safeBranchName)
	if _, err := os.Stat(localDir); err == nil {
		log.Printf("Removing local directory: %s", localDir)
		if err := os.RemoveAll(localDir); err != nil {
			return fmt.Errorf("failed to remove local directory %s: %w", localDir, err)
		}
		log.Printf("Successfully removed local directory: %s", localDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking local directory %s: %w", localDir, err)
	}

	return nil
}
