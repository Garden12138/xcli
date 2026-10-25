package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/spf13/cobra"
)

func (a *app) newModelClientCommand(stateDir *string) *cobra.Command {
	group := &cobra.Command{Use: "client", Short: "Manage independent local application tokens without printing secrets"}
	for _, rotate := range []bool{false, true} {
		var path string
		var scope []string
		var remote, consent bool
		var rpm int
		name := "issue"
		if rotate {
			name = "rotate"
		}
		command := &cobra.Command{Use: name + " <app-id>", Short: name + " a token into a new private file", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			if path == "" {
				return errors.New("--token-file is required; tokens are never printed")
			}
			store, err := modelstore.New(*stateDir)
			if err != nil {
				return err
			}
			client := model.Client{ID: args[0], AllowedModels: scope, AllowRemote: remote, RequireConsent: consent, RequestsPerMinute: rpm}
			if rotate {
				clients, err := store.Clients()
				if err != nil {
					return err
				}
				found := false
				for _, old := range clients {
					if old.ID == args[0] {
						client = old
						found = true
						break
					}
				}
				if !found {
					return errors.New("rotation target does not exist")
				}
			} else {
				cfg, _, err := modelstore.Load(a.configPath)
				if err != nil {
					return err
				}
				for _, id := range scope {
					if _, ok := cfg.Models[id]; !ok {
						return errors.New("client scope must name configured model aliases")
					}
				}
			}
			// Refuse an existing file or symlink. Its directory must be private.
			if err := modelstore.PrivateDirectory(filepath.Dir(path)); err != nil {
				return errors.New("token file parent must be a private directory (0700)")
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return errors.New("token file must be new and writable")
			}
			token, err := store.Issue(client, rotate)
			if err != nil {
				f.Close()
				os.Remove(path)
				return err
			}
			_, writeErr := fmt.Fprintln(f, token)
			syncErr := f.Sync()
			closeErr := f.Close()
			if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
				_ = store.Revoke(client.ID)
				_ = os.Remove(path)
				return errors.New("private token delivery failed; issued token revoked")
			}
			fmt.Fprintf(c.OutOrStdout(), "Client %s token saved to the requested private file\n", client.ID)
			return nil
		}}
		command.Flags().StringVar(&path, "token-file", "", "new private delivery file; transfer to the application's credential store")
		if !rotate {
			command.Flags().StringSliceVar(&scope, "model", nil, "allowed model aliases (repeat or comma separate)")
			command.Flags().BoolVar(&remote, "allow-remote", false, "allow this client to use remote or unknown inference")
			command.Flags().BoolVar(&consent, "require-consent", false, "require per-request cloud consent (mandatory for Chat Insight)")
			command.Flags().IntVar(&rpm, "requests-per-minute", 60, "per-client request limit")
		}
		group.AddCommand(command)
	}
	group.AddCommand(&cobra.Command{Use: "list", Short: "List client identities and scopes without token hashes", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		clients, err := store.Clients()
		if err != nil {
			return err
		}
		return encodeJSON(c.OutOrStdout(), clients)
	}}, &cobra.Command{Use: "revoke <app-id>", Short: "Revoke a client token immediately for new requests", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		if err = store.Revoke(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(c.OutOrStdout(), "Client %s revoked\n", args[0])
		return nil
	}})
	return group
}
