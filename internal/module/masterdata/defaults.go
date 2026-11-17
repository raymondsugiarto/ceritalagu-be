package masterdata

// Defaults is the starter option list loaded by `server masterdata import-defaults`. Admins edit
// or archive rows afterwards; the import never overwrites an existing code.
func Defaults() []Item {
	var out []Item
	add := func(k Kind, rows [][3]string) {
		for i, r := range rows {
			out = append(out, Item{Kind: k, Code: r[0], Name: r[1], Hint: r[2], SortOrder: (i + 1) * 10, IsActive: true})
		}
	}
	add(KindRecipientType, [][3]string{
		{"partner", "Pasangan", "Sung to a romantic partner (girlfriend/boyfriend): intimate, warm, first person."},
		{"spouse", "Suami/Istri", "Sung to a husband or wife: lifelong commitment, shared life, mature love."},
		{"mother", "Ibu", "Sung by a child to their mother: gratitude, sacrifice, tenderness."},
		{"father", "Ayah", "Sung by a child to their father: respect, quiet strength, gratitude."},
		{"child", "Anak", "Sung by a parent to their child: pride, protection, hopes for the future."},
		{"friend", "Sahabat", "Sung to a best friend: loyalty, shared adventures, playful warmth. Not romantic."},
		{"sibling", "Saudara", "Sung to a brother or sister: growing up together, teasing, unconditional support."},
		{"coworker", "Rekan Kerja", "Sung to a colleague: appreciation and teamwork, friendly and professional. Not romantic."},
		{"teacher", "Guru", "Sung by a student to their teacher: gratitude for guidance, patience and lessons that last a lifetime. Respectful, not romantic."},
		{"self", "Diri Sendiri", "A song to oneself: self-love, encouragement, growth."},
	})
	add(KindOccasion, [][3]string{
		{"birthday", "Ulang Tahun", "Birthday celebration: joyful, wishes for the year ahead."},
		{"anniversary", "Anniversary", "Relationship anniversary: look back on the journey together and forward to more."},
		{"wedding", "Pernikahan", "Wedding: vows, promise of forever, celebratory and emotional."},
		{"proposal", "Lamaran", "Marriage proposal: build up to asking them to spend life together."},
		{"parents_day", "Hari Ibu/Ayah", "Mother's or Father's Day: honour and thank the parent."},
		{"graduation", "Wisuda", "Graduation: pride in hard work, excitement for the next chapter."},
		{"apology", "Permintaan Maaf", "Apology: sincere regret, humility, hope to make things right."},
		{"thank_you", "Terima Kasih", "Thank-you song: specific gratitude for what they did."},
		{"farewell", "Perpisahan", "Farewell: bittersweet goodbye, cherished memories, good wishes."},
		{"valentine", "Valentine", "Valentine's Day: romantic, sweet, affectionate."},
		{"just_because", "Hanya Ingin Bilang Sayang", "No special occasion: a heartfelt reminder of how much they mean."},
	})
	add(KindGenre, [][3]string{
		{"pop", "Pop", "modern pop, catchy chorus, polished production"},
		{"ballad", "Ballad", "emotional ballad, piano and strings, slow tempo"},
		{"acoustic", "Acoustic", "acoustic guitar, warm intimate vocals, light percussion"},
		{"rnb", "R&B", "smooth R&B, soulful vocals, groovy bass, mid tempo"},
		{"rock", "Rock", "pop rock, electric guitars, driving drums, anthemic chorus"},
		{"jazz", "Jazz", "light jazz, swing feel, piano trio, mellow"},
		{"dangdut", "Dangdut", "Indonesian dangdut, kendang drums, suling flute, danceable"},
		{"keroncong", "Keroncong", "Indonesian keroncong, ukulele cak-cuk, cello pizzicato, nostalgic"},
		{"edm", "EDM", "upbeat EDM, synth leads, build-up and drop"},
		{"hiphop", "Hip-Hop", "hip-hop beat, rhythmic verses, melodic hook"},
		{"country", "Country", "country pop, acoustic guitar, storytelling"},
		{"lullaby", "Lullaby", "gentle lullaby, soft vocals, music box and soft piano, very slow"},
	})
	add(KindMood, [][3]string{
		{"romantic", "Romantis", "Romantic: tender, intimate, full of affection and longing."},
		{"cheerful", "Ceria", "Cheerful: bright, playful and happy, light-hearted imagery."},
		{"touching", "Haru", "Touching: emotional and moving, gentle build that can bring tears."},
		{"funny", "Lucu", "Funny: witty and playful, light teasing and humorous details, still affectionate."},
		{"upbeat", "Semangat", "Upbeat: energetic and motivating, driving rhythm, positive drive."},
		{"warm", "Hangat", "Warm: cozy and comforting, sincere gratitude and closeness, like a heartfelt hug. Mid tempo, soft and intimate."},
		{"inspiring", "Inspiratif", "Inspiring: uplifting and hopeful, celebrates their strength and growth, encourages chasing dreams. Builds to an anthemic chorus."},
	})
	return out
}
