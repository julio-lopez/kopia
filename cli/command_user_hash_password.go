package cli

import (
	"context"

	"github.com/alecthomas/kingpin/v2"
	"github.com/kopia/kopia/repo"
)

type commandServerUserHashPassword struct {
	userAskPassword bool
	password        string

	out textOutput
}

func (c *commandServerUserHashPassword) setup(svc appServices, parent commandParent) {
	var cmd *kingpin.CmdClause

	cmd = parent.Command("hash-password", "Hash a user password that can be passed to the 'server user add/set' command").Alias("hash")

	cmd.Flag("ask-password", "Ask for user password").BoolVar(&c.userAskPassword)
	cmd.Flag("user-password", "Password").StringVar(&c.password)

	// cmd.Action(svc.baseActionWithContext(c.runServerUserHashPassword))
	cmd.Action(svc.repositoryWriterAction(c.runServerUserHashPassword))

	c.out.setup(svc)
}

func (c *commandServerUserHashPassword) runServerUserHashPassword(ctx context.Context, rep repo.RepositoryWriter) error {
	// semantics:
	// when user-password is empty, ask for password
	// when ask-password is true, ask for password regardless of whether or not
	// a password was specified? this is confusing. Maybe remove this flag.
	return nil
}
