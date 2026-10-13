package main

import "strconv"

// The directory is generated from the row index, so the 100,000 people cost
// nothing until a row is shown: a real app would page them in from a store.

// Count is the number of people in the directory.
const Count = 100_000

var (
	firstNames = [...]string{
		"Ana", "Ben", "Chloé", "Dmitri", "Elena", "Farid", "Grace", "Hiro", "Inés", "Jonas",
		"Kavya", "Liam", "Mei", "Nadia", "Omar", "Priya", "Quentin", "Rosa", "Samuel", "Tomás",
		"Uma", "Viktor", "Wen", "Ximena", "Yusuf", "Zoe", "Amara", "Bruno", "Clara", "Diego",
		"Esther", "Felix", "Greta", "Hugo", "Isla", "Jamal", "Kenji", "Lucía", "Mateo", "Nora",
	}
	lastNames = [...]string{
		"Abbott", "Bauer", "Castillo", "Dubois", "Eriksen", "Fischer", "García", "Hayashi", "Ibrahim", "Jensen",
		"Kowalski", "Lindqvist", "Moreau", "Novak", "Okafor", "Petrov", "Quinn", "Rossi", "Silva", "Tanaka",
		"Ueda", "Vargas", "Weber", "Xu", "Yilmaz", "Zimmermann", "Almeida", "Brennan", "Chen", "Dahl",
		"Ekström", "Ferreira", "Gallagher", "Hansen", "Ivanova", "Jovanović", "Kim", "López", "Müller", "Nakamura",
	}
	roles = [...]string{
		"Product designer", "Staff engineer", "Data analyst", "Support lead", "Engineering manager", "Recruiter",
		"Account executive", "Site reliability engineer", "UX researcher", "Finance partner", "Developer advocate", "Technical writer",
	}
	cities = [...]string{
		"Lisbon", "Berlin", "Montréal", "Tokyo", "Nairobi", "São Paulo", "Toronto", "Seoul",
		"Madrid", "Oslo", "Bogotá", "Melbourne", "Istanbul", "Mumbai", "Dublin", "Mexico City",
	}
	notes = [...]string{
		"Out of office until Monday; reach the team channel for anything urgent.",
		"Leads the accessibility guild and reviews every new component before it ships. Ask about the keyboard checklist.",
		"Prefers asynchronous updates. Weekly office hours on Thursdays at 15:00 for questions about the data platform, the quarterly planning documents or onboarding.",
		"New hire: say hello!",
		"Moving to the Lisbon office in the spring, working remotely in the meantime from a timezone that overlaps with both Europe and the Americas.",
	}
	// avatarColors are the gradients of the avatars.
	avatarColors = [...]string{
		"from-violet-400 to-indigo-500", "from-sky-400 to-cyan-500", "from-amber-400 to-orange-500",
		"from-rose-400 to-pink-500", "from-emerald-400 to-teal-500", "from-fuchsia-400 to-purple-500",
		"from-lime-400 to-green-500", "from-slate-400 to-zinc-500",
	}
)

// Person is one row of the directory.
type Person struct {
	Name, Initials, Role, City, Colors string
	// Note is a longer text that wraps, on one row in seven.
	Note string
}

// HasNote reports whether the person at index i has a note.
func HasNote(i int) bool { return i%7 == 3 }

// PersonAt returns the person at index i.
func PersonAt(i int) Person {
	first := firstNames[i%len(firstNames)]
	last := lastNames[(i/len(firstNames)+i*7)%len(lastNames)]
	p := Person{
		Name:     first + " " + last,
		Initials: initial(first) + initial(last),
		Role:     roles[(i*5+i/3)%len(roles)],
		City:     cities[(i*3+i/11)%len(cities)],
		Colors:   avatarColors[(i*5)%len(avatarColors)],
	}
	if HasNote(i) {
		p.Note = notes[(i/7)%len(notes)]
	}
	return p
}

// initial returns the first letter of s.
func initial(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// Grouped formats n with thousands separators: 100000 is "100,000".
func Grouped(n int) string {
	if n < 0 {
		return "-" + Grouped(-n)
	}
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
