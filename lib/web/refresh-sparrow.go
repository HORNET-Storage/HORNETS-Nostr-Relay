package web

import (
	"fmt"
	"log"
	"os/exec"
)

func refreshSparrow() error {
	log.Println("Refreshing Sparrow wallet")
	// Command to run the bash script
	cmd := exec.Command("/bin/bash", "/home/siphiwe/my_go/GitNestr_Latest/new_hornet_storage/hornet-storage/refresh-sparrow.sh")

	// Run the command and capture any errors and outputs
	output, err := cmd.CombinedOutput()

	// Always print the output regardless of success or error
	outputStr := string(output)
	log.Println("Script output:", outputStr)

	if err != nil {
		// Include output in the error for more context
		return fmt.Errorf("failed to refresh Sparrow wallet: %v, output: %s", err, outputStr)
	}

	fmt.Println("Sparrow wallet refreshed successfully")
	return nil
}
