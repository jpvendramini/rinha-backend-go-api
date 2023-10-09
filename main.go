package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
	"regexp"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"github.com/jackc/pgtype"
	"errors"
	"database/sql/driver"
)

type Person struct {
	ID         uuid.UUID 	  `json:"id" gorm:"primaryKey"`
	Apelido    string    	  `json:"apelido" gorm:"size:32;unique;not null"`
	Nome       *string    	  `json:"nome" gorm:"size:100;not null"`
	Nascimento *string    	  `json:"nascimento" gorm:"size:10;not null"`
	Stack      pgtype.JSONB   `json:"stack" gorm:"type:jsonb;default:'[]'"`
}

// Scan implements the sql.Scanner interface for the Stack field
func (p *Person) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}
	return p.Stack.UnmarshalJSON(bytes)
}

// Value implements the driver.Valuer interface for the Stack field
func (p *Person) Value() (driver.Value, error) {
	if p.Stack.Status == pgtype.Null {
		return nil, nil
	}

	// Unmarshal to a []interface{}
	var v []interface{}
	if err := json.Unmarshal(p.Stack.Bytes, &v); err != nil {
		return nil, err
	}

	// If the JSONB value is an empty array, return an empty JSONB value
	if len(v) == 0 {
		return []byte("[]"), nil
	}

	return p.Stack.MarshalJSON()
}

var db *gorm.DB

func main() {
	// Inicializar o DB
	dsn := "user=postgres password=2309 dbname=people host=db port=5432 sslmode=disable"
	var err error

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		PrepareStmt: true,
	})

	if err != nil {
		panic(fmt.Sprintf("Erro ao conectar ao banco de dados: %v", err))
	}

	sqlDB, err := db.DB()

	if err != nil {
		panic(fmt.Sprintf("Erro ao obter o banco de dados SQL: %v", err))
	}

	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(time.Hour)

	db.Exec("ALTER TABLE people ALTER COLUMN stack TYPE jsonb USING jsonb('[]')")

	if err := db.AutoMigrate(&Person{}); err != nil {
		panic(fmt.Sprintf("Erro ao migrar o modelo para o banco de dados: %v", err))
	}

	r := gin.Default()

	r.POST("/pessoas", CreatePerson)
	r.GET("/pessoas/:id", FindById)
	r.GET("/pessoas", SearchPeople)
	r.GET("/contagem-pessoas", CountPeople)

	r.Run(":3000")
}

func CreatePerson(c *gin.Context) {
	var person Person
	if err := c.ShouldBindJSON(&person); err != nil {
		c.Status(400)
		return
	}

	person.Apelido = strings.TrimSpace(person.Apelido)
	if (len(person.Apelido) == 0 || utf8.RuneCountInString(person.Apelido) > 32) {
		c.Status(422)
		return
	}

	if(person.Nome == nil){
		c.Status(422)
		return
	}

	*person.Nome = strings.TrimSpace(*person.Nome)
	if utf8.RuneCountInString(*person.Nome) > 100 {
		c.Status(422)
		return
	}

	if(person.Nascimento == nil){
		c.Status(422)
		return
	}

	var re = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	if (!re.MatchString(*person.Nascimento) || utf8.RuneCountInString(*person.Nascimento) != 10) {
		c.Status(400)
		return
	}

	if _, err := time.Parse("2006-01-02", *person.Nascimento); err != nil {
		c.Status(422)
    	return
	}

	// // Check if "Stack" is undefined and set it to an empty JSON array
	// if person.Stack.Status == pgtype.Undefined {
	// 	person.Stack.Status = pgtype.Present
	// 	if err := person.Stack.Set([]interface{}{}); err != nil {
	// 		c.Status(422)
	// 		return
	// 	}
	// }

	// Check each element in Stack
	var stackItems []interface{}
	if err := person.Stack.AssignTo(&stackItems); err != nil {
		c.Status(400)
		return
	}

	// Check each element in Stack
	for _, stackItem := range stackItems {
		if stackItem == nil {
			c.Status(422)
			return
		}
		stackItemStr, ok := stackItem.(string)
		if !ok {
			c.Status(400)
			return
		}
		if utf8.RuneCountInString(stackItemStr) > 32 {
			c.Status(422)
			return
		}
	}

	person.ID = uuid.New()

	result := db.Create(&person)

	if result.Error != nil {
		c.Status(422)
		return
	}

	c.Header("Content-Type", "application/json")
  	c.Header("Location", fmt.Sprintf("/pessoas/%s", person.ID))
	c.Status(201)
}

func FindById(c *gin.Context) {
	id := c.Param("id")

	var person Person
	result := db.First(&person, "id = ?", id)

	if result.Error != nil {
		c.Status(404)
		return
	}

	c.Header("Content-Type", "application/json")
	c.JSON(200, person)
}

func SearchPeople(c *gin.Context) {
	t := c.Query("t")

	if len(t) == 0 {
		c.JSON(400, gin.H{"error": "O termo de busca é obrigatório."})
		return
	}

	var people []Person
	query := `SELECT * FROM people WHERE apelido ILIKE $1 OR nome ILIKE $1 OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(stack) AS stack_item WHERE stack_item ILIKE $1) LIMIT 50`
	result := db.Raw(query, "%"+t+"%").Scan(&people)
	if result.Error != nil {
		fmt.Println("Database Error:", result.Error)
		c.Status(500) // Internal Server Error
		return
	}
	
	if len(people) == 0 {
		c.JSON(200, []Person{})
		return
	}

	c.JSON(200, people)
}

func CountPeople(c *gin.Context) {
	var count int64
	db.Model(&Person{}).Count(&count)
	c.String(200, "%d", count)
}