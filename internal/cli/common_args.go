package cli

type commonArgs struct {
	Server string `arg:"-s,--server" placeholder:"URL" help:"remote Grepple service URL"`
}
