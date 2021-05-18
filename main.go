package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/oauth2"

	"github.com/Luzifer/rconfig"
	"github.com/google/go-github/github"
)

func listTeams(client *github.Client, org string) ([]*github.Team, error) {
	page := 1
	teams := []*github.Team{}
	for {
		teamsFromPage, resp, err := client.Organizations.ListTeams(
			org,
			&github.ListOptions{Page: page},
		)
		if err != nil {
			return nil, fmt.Errorf("Error: on page %d fetching teams failed with %s", page, err)
		}
		teams = append(teams, teamsFromPage...)
		if resp.NextPage == 0 || resp.LastPage == 0 {
			// We're done here.
			break
		}
		page = resp.NextPage
	}
	return teams, nil
}

func getTeam(client *github.Client, org string, name string) (*github.Team, error) {
	teams, err := listTeams(client, org)
	if err != nil {
		return nil, fmt.Errorf("fetching teams failed: %s", err)
	}

	for _, team := range teams {
		if *team.Name == name {
			return team, nil
		}
	}
	return nil, errors.New("Team not found")
}

func getMembers(client *github.Client, team *github.Team) ([]*github.User, error) {
	page := 1
	users := []*github.User{}

	for {
		usersFromPage, resp, err := client.Organizations.ListTeamMembers(
			*team.ID,
			&github.OrganizationListTeamMembersOptions{ListOptions: github.ListOptions{Page: page}},
		)
		if err != nil {
			return nil, fmt.Errorf("Error: on page %d fetching team members failed with %s", page, err)
		}
		users = append(users, usersFromPage...)
		if resp.NextPage == 0 || resp.LastPage == 0 {
			// We're done here.
			break
		}
		page = resp.NextPage
	}
	return users, nil
}

func getClient(token string) *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return client
}

func writeKeys(client *github.Client, w io.Writer, users []*github.User) error {
	for _, user := range users {
		keys, _, err := client.Users.ListKeys(*user.Login, nil)
		if err != nil {
			return fmt.Errorf("Error when fetching keys: %s", err)
		}
		for _, key := range keys {
			if key.Key == nil {
				return fmt.Errorf("Key is null pointer for %s", *user.Login)
			}
			fmt.Fprintln(w, *key.Key)
		}
	}
	return nil
}

func main() {
	cfg := struct {
		TeamName     string `flag:"team" description:"Team to look for"`
		GithubToken  string `flag:"token" env:"GITHUB_TOKEN" description:"Github token"`
		Organization string `flag:"org" description:"Github Organization"`
		OutFile      string `flag:"out,o" default:"-" description:"Output file"`
		Append       bool   `flag:"append" description:"Append ssh keys"`
	}{}
	if err := rconfig.Parse(&cfg); err != nil {
		log.Fatalf("Error parsing cli flags: %s", err)
	}

	client := getClient(cfg.GithubToken)

	team, err := getTeam(client, cfg.Organization, cfg.TeamName)
	if err != nil {
		log.Fatalf("Getting team failed: %s", err)
	}
	members, err := getMembers(client, team)
	if err != nil {
		log.Fatalf("fetching members failed: %s", err)
	}

	switch cfg.OutFile {
	case "-":
		if err := writeKeys(client, os.Stdout, members); err != nil {
			log.Fatalf("Unable to write keys: %s", err)
		}
	default:
		o, err := os.Create(cfg.OutFile + ".tmp")
		if err != nil {
			log.Fatalf("Unable to open output file: %s", err)
		}

		if cfg.Append {
			current, err := ioutil.ReadFile(cfg.OutFile)
			/* if we got something write it */
			if err == nil {
				o.Write(current)
			}
		}

		if err := writeKeys(client, o, members); err != nil {
			log.Fatalf("Unable to write keys: %s", err)
		}

		o.Close()

		if err := os.Rename(cfg.OutFile+".tmp", cfg.OutFile); err != nil {
			log.Fatalf("Moving tmp file to %s failed: %s", cfg.OutFile, err)
		}
	}
}
