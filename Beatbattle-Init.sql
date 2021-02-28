-- --------------------------------------------------------
-- Host:                         127.0.0.1
-- Server version:               8.0.22 - MySQL Community Server - GPL
-- Server OS:                    Win64
-- HeidiSQL Version:             11.1.0.6116
-- --------------------------------------------------------

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET NAMES utf8 */;
/*!50503 SET NAMES utf8mb4 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;


-- Dumping database structure for beatbattle3
CREATE DATABASE IF NOT EXISTS `beatbattle3` /*!40100 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci */ /*!80016 DEFAULT ENCRYPTION='N' */;
USE `beatbattle3`;

-- Dumping structure for table beatbattle3.ads
CREATE TABLE IF NOT EXISTS `ads` (
  `id` int NOT NULL AUTO_INCREMENT,
  `active` tinyint NOT NULL DEFAULT '0',
  `startdate` datetime NOT NULL,
  `enddate` datetime NOT NULL,
  `url` text NOT NULL,
  `clicks` int NOT NULL DEFAULT '0',
  `user_id` int NOT NULL,
  `image` text NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_user_id_ad_idx` (`user_id`),
  CONSTRAINT `fk_user_id_ad` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.battles
CREATE TABLE IF NOT EXISTS `battles` (
  `id` int NOT NULL AUTO_INCREMENT,
  `title` varchar(256) NOT NULL,
  `rules` text NOT NULL,
  `deadline` datetime NOT NULL,
  `attachment` text,
  `password` varchar(64) DEFAULT NULL,
  `results` tinyint DEFAULT '0',
  `user_id` int NOT NULL,
  `type` varchar(16) NOT NULL DEFAULT 'beat',
  `voting_deadline` datetime DEFAULT NULL,
  `maxvotes` int DEFAULT '1',
  `winner_id` int NOT NULL DEFAULT '0',
  `settings_id` int DEFAULT '0',
  `tags` varchar(256) DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.battles_tags
CREATE TABLE IF NOT EXISTS `battles_tags` (
  `battle_id` int NOT NULL,
  `tag_id` int NOT NULL,
  KEY `fk_challenges_tags_tag_id_idx` (`tag_id`),
  KEY `fk_challenges_tags_challenges_idx` (`battle_id`) USING BTREE,
  CONSTRAINT `fk_challenges_tags_challenge_id` FOREIGN KEY (`battle_id`) REFERENCES `battles` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_challenges_tags_tag_id` FOREIGN KEY (`tag_id`) REFERENCES `tags` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.battle_settings
CREATE TABLE IF NOT EXISTS `battle_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `logo` text,
  `background` text,
  `show_users` tinyint DEFAULT '0',
  `show_entries` tinyint DEFAULT '0',
  `tracking_id` varchar(128) DEFAULT '',
  `private` tinyint DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.beats
CREATE TABLE IF NOT EXISTS `beats` (
  `id` int NOT NULL AUTO_INCREMENT,
  `url` text NOT NULL,
  `votes` int NOT NULL DEFAULT '0',
  `battle_id` int NOT NULL,
  `user_id` int NOT NULL,
  `voted` tinyint NOT NULL DEFAULT '0',
  `placement` int DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `fk_user_id_beats_idx` (`user_id`),
  KEY `fk_challenge_id_beats` (`battle_id`) USING BTREE,
  CONSTRAINT `fk_challenge_id_beats` FOREIGN KEY (`battle_id`) REFERENCES `battles` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_id_beats` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.feedback
CREATE TABLE IF NOT EXISTS `feedback` (
  `id` int NOT NULL AUTO_INCREMENT,
  `feedback` varchar(512) NOT NULL,
  `user_id` int NOT NULL,
  `beat_id` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_feedback_user_idx` (`user_id`),
  KEY `fk_feedback_beat_idx` (`beat_id`),
  CONSTRAINT `fk_feedback_beat` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.likes
CREATE TABLE IF NOT EXISTS `likes` (
  `user_id` int NOT NULL,
  `beat_id` int NOT NULL,
  `battle_id` int NOT NULL,
  KEY `fk_user_likes_idx` (`user_id`),
  KEY `fk_beat_likes_idx` (`beat_id`),
  KEY `likes_idx_user_id_beat_id` (`user_id`,`beat_id`),
  KEY `fk_challenge_likes` (`battle_id`) USING BTREE,
  CONSTRAINT `fk_beat_likes` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_challenge_likes` FOREIGN KEY (`battle_id`) REFERENCES `battles` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_likes` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.tags
CREATE TABLE IF NOT EXISTS `tags` (
  `id` int NOT NULL AUTO_INCREMENT,
  `tag` varchar(64) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `tag_UNIQUE` (`tag`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.users
CREATE TABLE IF NOT EXISTS `users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `provider` varchar(64) NOT NULL,
  `provider_id` varchar(64) NOT NULL,
  `nickname` varchar(64) NOT NULL,
  `access_token` char(60) DEFAULT '',
  `expiry` datetime DEFAULT CURRENT_TIMESTAMP,
  `flair` varchar(64) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

-- Dumping structure for table beatbattle3.votes
CREATE TABLE IF NOT EXISTS `votes` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(64) NOT NULL,
  `battle_id` int NOT NULL,
  `beat_id` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_beat_id_idx` (`beat_id`),
  KEY `fk_customer_id_idx` (`battle_id`) USING BTREE,
  CONSTRAINT `fk_beat_id` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_challenge_id` FOREIGN KEY (`battle_id`) REFERENCES `battles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Data exporting was unselected.

/*!40101 SET SQL_MODE=IFNULL(@OLD_SQL_MODE, '') */;
/*!40014 SET FOREIGN_KEY_CHECKS=IF(@OLD_FOREIGN_KEY_CHECKS IS NULL, 1, @OLD_FOREIGN_KEY_CHECKS) */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;
