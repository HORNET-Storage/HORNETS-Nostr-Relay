package web

import (
	"log"

	"golang.org/x/crypto/bcrypt"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func handleSignUp(c *fiber.Ctx) error {
	log.Println("Sign-up request received")
	var signUpPayload struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		Npub      string `json:"npub"`
	}

	if err := c.BodyParser(&signUpPayload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(signUpPayload.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	user := types.User{
		FirstName: signUpPayload.FirstName,
		LastName:  signUpPayload.LastName,
		Email:     signUpPayload.Email,
		Password:  string(hashedPassword),
		Npub:      signUpPayload.Npub,
	}

	if err := db.Create(&user).Error; err != nil {
		log.Printf("Failed to create user: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User created successfully",
	})
}
