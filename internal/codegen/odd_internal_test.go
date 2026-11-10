package codegen

import "testing"

func TestOddNamesUnit(t *testing.T) {
	for in, want := range map[string]string{"_": "X", "日本語": "X日本語", "1st": "X1st", "user_id": "UserID", "ñ": "Ñ", "{}": "X"} {
		if got := camel(in); got != want {
			t.Errorf("camel(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"plain": "`db:\"plain\"`", "a`b": "\"db:\\\"a`b\\\"\""} {
		if got := dbTag(in); got != want {
			t.Errorf("dbTag(%q) = %s, want %s", in, got, want)
		}
	}
	taken := map[string][]byte{"a_b.go": nil, "enums.go": nil}
	for in, want := range map[string]string{"a_b": "a_b_2", "_": "t_", "": "t", "_x": "t_x", "enums": "enums_2", "fresh": "fresh"} {
		if got := uniqueFile(in, taken); got != want {
			t.Errorf("uniqueFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSingularRowNames(t *testing.T) {
	for in, want := range map[string]string{
		"Users": "User", "Orders": "Order", "Categories": "Category", "Addresses": "Address", "Boxes": "Box",
		"Statuses": "Status", "OrderStatuses": "OrderStatus", "Buses": "Bus", "Viruses": "Virus",
		"Aliases": "Alias", "Analyses": "Analysis", "People": "Person", "Children": "Child",
		"Indices": "Index", "Quizzes": "Quiz", "Heroes": "Hero",
		"News": "News", "Series": "Series", "Data": "Data", "Metadata": "Metadata", "UserData": "UserData",
		"Abuses": "Abuse", "Causes": "Cause", "Houses": "House", "Responses": "Response", "Databases": "Database",
		"Status": "Status", "Bus": "Bus", "UserIDs": "UserID", "Shoes": "Shoe",
		"Movies": "Movie", "Cookies": "Cookie", "UserCaches": "UserCache", "Matches": "Match", "Batches": "Batch",
		"Menus": "Menu", "Emojis": "Emoji", "Shelves": "Shelf", "Ties": "Tie",
		"Alias": "Alias", "Canvas": "Canvas", "Atlas": "Atlas", "Bias": "Bias", "Chaos": "Chaos", "Axis": "Axis",
		"OrderAlias": "OrderAlias",
	} {
		if got := singular(in); got != want {
			t.Errorf("singular(%q) = %q, want %q", in, got, want)
		}
	}
}
