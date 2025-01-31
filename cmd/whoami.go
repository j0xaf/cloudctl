package cmd

import (
	"fmt"
	"time"

	"github.com/metal-stack/metal-lib/jwt/sec"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "shows current user",
	Long:  "shows the current user, that will be used to authenticate commands.",
	RunE: func(cmd *cobra.Command, args []string) error {

		authContext, err := getAuthContext(viper.GetString("kubeConfig"))
		if err != nil {
			return err
		}

		user, parsedClaims, err := sec.ParseTokenUnvalidatedUnfiltered(authContext.IDToken)
		if err != nil {
			return err
		}

		fmt.Printf("UserId: %s\n", user.Name)
		if user.Tenant != "" {
			fmt.Printf("Tenant: %s\n", user.Tenant)
		}
		if user.Issuer != "" {
			fmt.Printf("Issuer: %s\n", user.Issuer)
		}
		fmt.Printf("Groups:\n")
		for _, g := range user.Groups {
			fmt.Printf(" %s\n", g)
		}
		fmt.Printf("Expires at %s\n", time.Unix(parsedClaims.ExpiresAt, 0).Format("Mon Jan 2 15:04:05 MST 2006"))

		return nil
	},
	PreRun: bindPFlags,
}
