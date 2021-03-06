package dao

//GetCourseNumStudents returns the number of students enrolled in the course
func GetCourseNumStudents(courseID uint) (int, error) {
	var res int
	err := DB.Raw(`SELECT count(*) FROM course_users WHERE course_id = ?`, courseID).Scan(&res).Error
	return res, err
}

//GetCourseNumVodViews returns the sum of vod views of a course
func GetCourseNumVodViews(courseID uint) (int, error) {
	var res int
	err := DB.Raw(`SELECT SUM(stats.viewers) FROM stats
		JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ? AND live = 0`, courseID).Scan(&res).Error
	return res, err
}

//GetCourseNumLiveViews returns the sum of live views of a course based on the maximum views per lecture
func GetCourseNumLiveViews(courseID uint) (int, error) {
	var res int
	err := DB.Raw(`WITH views_per_stream AS (SELECT MAX(stats.viewers) AS y
		FROM stats
        	JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ?
            AND stats.live = 1
        GROUP BY stats.stream_id)
		SELECT SUM(y)
			FROM views_per_stream`, courseID).Scan(&res).Error
	return res, err
}

//GetCourseStatsWeekdays returns the days and their sum of vod views of a course
func GetCourseStatsWeekdays(courseID uint) ([]Stat, error) {
	var res []Stat
	err := DB.Raw(`SELECT DAYNAME(stats.time) AS x, SUM(stats.viewers) as y
		FROM stats
			JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ? AND stats.live = 0
		GROUP BY DAYOFWEEK(stats.time);`,
		courseID).Scan(&res).Error
	return res, err
}

//GetCourseStatsHourly returns the hours with most vod viewing activity of a course
func GetCourseStatsHourly(courseID uint) ([]Stat, error) {
	var res []Stat
	err := DB.Raw(`SELECT HOUR(stats.time) AS x, SUM(stats.viewers) as y
		FROM stats
			JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ? AND stats.live = 0
		GROUP BY HOUR(stats.time);`,
		courseID).Scan(&res).Error
	return res, err
}

func GetStudentActivityCourseStats(courseID uint, live bool) ([]Stat, error) {
	var res []Stat
	if live {
		err := DB.Raw(`SELECT CONCAT(YEAR(stats.time), ' w', WEEK(stats.time)) AS x, MAX(stats.viewers) AS y
		FROM stats
        	JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ? AND stats.live = 1
		GROUP BY year(stats.time), week(stats.time)
		ORDER BY stats.time;`,
			courseID).Scan(&res).Error
		return res, err
	} else {
		err := DB.Raw(`SELECT CONCAT(YEAR(stats.time), ' w', WEEK(stats.time)) AS x, SUM(stats.viewers) AS y
		FROM stats
        	JOIN streams s ON s.id = stats.stream_id
		WHERE s.course_id = ? AND stats.live = 0
		GROUP BY year(stats.time), week(stats.time)
		ORDER BY stats.time;`,
			courseID).Scan(&res).Error
		return res, err
	}
}

//Stat key value struct that is parsable by Chart.js without further modifications.
//See https://www.chartjs.org/docs/master/general/data-structures.html
type Stat struct {
	X string `json:"x"` // label for stat
	Y int    `json:"y"` // value for stat
}
