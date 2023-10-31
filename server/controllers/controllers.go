package controllers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/tashima42/ellp-manager/server/database"
	"github.com/tashima42/ellp-manager/server/hash"
	"go.uber.org/zap"
)

type Controller struct {
	DB       *sqlx.DB
	Logger   *zap.SugaredLogger
	Validate *validator.Validate
}

func (cr *Controller) CreateUser(c *fiber.Ctx) error {
	requestID := fmt.Sprintf("%+v", c.Locals("requestid"))
	user := &database.User{}
	cr.Logger.Info(requestID, " unmarshal request body")
	if err := json.Unmarshal(c.Body(), user); err != nil {
		return err
	}

	if err := cr.Validate.Struct(user); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}

	cr.Logger.Info(requestID, " starting transaction")
	tx, err := cr.DB.BeginTxx(c.Context(), &sql.TxOptions{})
	if err != nil {
		return err
	}

	cr.Logger.Info(requestID, " looking for user with email "+user.Email)
	if _, err := database.GetUserByEmailTxx(tx, user.Email); err != nil {
		cr.Logger.Info(requestID, " error: "+err.Error())
		if !strings.Contains(err.Error(), "no rows in result set") {
			return err
		}
		cr.Logger.Info(requestID, " user doesn't exists, continue")
	} else {
		zap.Error(errors.New(requestID + " email was already registered"))
		return fiber.NewError(http.StatusConflict, "email "+user.Email+" already was registered")
	}

	cr.Logger.Info(requestID, " hashing password")
	hashedPassword, err := hash.Password(user.Password)
	if err != nil {
		return err
	}
	user.Password = hashedPassword

	cr.Logger.Info(requestID, " creating user")
	if err := database.CreateUserTxx(tx, user); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	cr.Logger.Info(requestID, " user created")
	return c.JSON(map[string]interface{}{"success": true})
}

func (cr *Controller) CreateDocument(c *fiber.Ctx) error {
	requestID := fmt.Sprintf("%+v", c.Locals("requestid"))
	document := &database.Document{}
	cr.Logger.Info(requestID, " unmarshal request body")
	if err := json.Unmarshal(c.Body(), document); err != nil {
		return err
	}

	if err := cr.Validate.Struct(document); err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}

	cr.Logger.Info(requestID, " starting transaction")
	tx, err := cr.DB.BeginTxx(c.Context(), &sql.TxOptions{})
	if err != nil {
		return err
	}

	cr.Logger.Info(requestID, " looking for user with id ", document.UserID)
	if _, err := database.GetUserByIDTxx(tx, document.UserID); err != nil {
		cr.Logger.Info(requestID, " error: "+err.Error())
		if strings.Contains(err.Error(), "no rows in result set") {
			return fiber.NewError(http.StatusBadRequest, "user not found")
		}
		return err
	}

	cr.Logger.Info(requestID, " looking for reviewer user with id ", document.ReviewerID)
	if _, err := database.GetUserByIDTxx(tx, document.ReviewerID); err != nil {
		cr.Logger.Info(requestID, " error: "+err.Error())
		if strings.Contains(err.Error(), "no rows in result set") {
			return fiber.NewError(http.StatusBadRequest, "user not found")
		}
		return err
	}

	cr.Logger.Info(requestID, " creating document")
	documentID, err := database.CreateDocumentTxx(tx, document)
	if err != nil {
		return err
	}
	if err := database.CreateLogTxx(tx, &database.Log{
		Action:      "create",
		DocumentID:  documentID,
		UserID:      document.ReviewerID,
		Description: "created document " + document.Name,
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	cr.Logger.Info(requestID, " document created")
	return c.JSON(map[string]interface{}{"success": true})
}
