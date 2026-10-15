//go:build !darwin

package hoststats

func platformAPFSCommand() apfsCommandFunc {
	return nil
}
