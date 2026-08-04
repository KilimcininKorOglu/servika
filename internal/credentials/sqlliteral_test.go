package credentials

import "testing"

// MariaDB rejects a placeholder in CREATE/ALTER USER ... IDENTIFIED BY, so the
// password is built into the statement text and escapeSQLString is the only
// thing standing between it and the surrounding literal.
//
// That escaping (covered by TestEscapeSQLStringEscapesQuotesAndBackslashes) is
// NOT what holds the property today: it assumes backslash is an escape
// character, which stops being true under sql_mode NO_BACKSLASH_ESCAPES. What
// actually holds is the validation pattern, which admits no character that can
// end a literal or open a new statement. These tests lock the pattern, so
// widening it fails here and forces the escaping question to be answered
// rather than assumed.
func TestPasswordPatternRejectsEveryLiteralBreakingCharacter(t *testing.T) {
	dangerous := []struct {
		name  string
		value string
	}{
		{name: "single quote", value: "pass'word234"},
		{name: "backslash", value: `pass\word234`},
		{name: "double quote", value: `pass"word234`},
		{name: "backtick", value: "pass`word234"},
		{name: "semicolon", value: "pass;word234"},
		{name: "newline", value: "password234\nDROP DATABASE panel"},
		{name: "carriage return", value: "password234\rDROP DATABASE panel"},
		{name: "NUL", value: "password234\x00"},
		{name: "escaped quote", value: `pass\'word234`},
		{name: "statement break", value: `x', 'y'); DROP DATABASE panel; -- `},
	}
	for _, test := range dangerous {
		t.Run(test.name, func(t *testing.T) {
			if mysqlPasswordPattern.MatchString(test.value) {
				t.Fatalf("mysqlPasswordPattern accepts %q, which can end the SQL literal it is built into", test.value)
			}
		})
	}

	if !mysqlPasswordPattern.MatchString(RandomPassword(24)) {
		t.Fatal("mysqlPasswordPattern rejects the panel's own generated password")
	}
}

func TestIdentifierPatternRejectsEveryQuotingCharacter(t *testing.T) {
	dangerous := []string{
		"db`name",
		"db'name",
		`db"name`,
		"db name",
		"db;name",
		"db.name",
		"db\nname",
		"db-name",
	}
	for _, value := range dangerous {
		if mysqlIdentifierPattern.MatchString(value) {
			t.Errorf("mysqlIdentifierPattern accepts %q, which can escape the backtick quoting it is built into", value)
		}
	}
	if !mysqlIdentifierPattern.MatchString("c_example_com_app") {
		t.Error("mysqlIdentifierPattern rejects an ordinary panel-generated identifier")
	}
}
