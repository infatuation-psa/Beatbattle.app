/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET NAMES utf8 */;
/*!50503 SET NAMES utf8mb4 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;


-- Dumping database structure for beatbattle
CREATE DATABASE IF NOT EXISTS `beatbattle` /*!40100 DEFAULT CHARACTER SET utf8mb4 */;
USE `beatbattle`;

-- Dumping structure for table beatbattle.ads
CREATE TABLE IF NOT EXISTS `ads` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `active` tinyint(4) NOT NULL DEFAULT 0,
  `startdate` datetime NOT NULL,
  `enddate` datetime NOT NULL,
  `url` text NOT NULL,
  `clicks` int(11) NOT NULL DEFAULT 0,
  `user_id` int(11) NOT NULL,
  `image` text NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_user_id_ad_idx` (`user_id`),
  CONSTRAINT `fk_user_id_ad` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.beats
CREATE TABLE IF NOT EXISTS `beats` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `url` text NOT NULL,
  `votes` int(11) NOT NULL DEFAULT 0,
  `challenge_id` int(11) NOT NULL,
  `user_id` int(11) NOT NULL,
  `voted` tinyint(4) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `fk_challenge_id_beats` (`challenge_id`),
  KEY `fk_user_id_beats_idx` (`user_id`),
  CONSTRAINT `fk_challenge_id_beats` FOREIGN KEY (`challenge_id`) REFERENCES `challenges` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_id_beats` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=8789 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.challenges
CREATE TABLE IF NOT EXISTS `challenges` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `title` varchar(256) NOT NULL,
  `rules` text NOT NULL,
  `deadline` datetime NOT NULL,
  `attachment` text DEFAULT NULL,
  `status` varchar(32) DEFAULT 'entry',
  `password` varchar(64) DEFAULT NULL,
  `user_id` int(11) NOT NULL,
  `type` varchar(16) NOT NULL DEFAULT 'beat',
  `group_id` int(11) DEFAULT 0,
  `voting_deadline` datetime DEFAULT NULL,
  `maxvotes` int(11) DEFAULT 1,
  `winner_id` int(11) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=215 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.challenges_tags
CREATE TABLE IF NOT EXISTS `challenges_tags` (
  `challenge_id` int(11) NOT NULL,
  `tag_id` int(11) NOT NULL,
  KEY `fk_challenges_tags_challenges_idx` (`challenge_id`),
  KEY `fk_challenges_tags_tag_id_idx` (`tag_id`),
  CONSTRAINT `fk_challenges_tags_challenge_id` FOREIGN KEY (`challenge_id`) REFERENCES `challenges` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_challenges_tags_tag_id` FOREIGN KEY (`tag_id`) REFERENCES `tags` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.feedback
CREATE TABLE IF NOT EXISTS `feedback` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `feedback` varchar(512) NOT NULL,
  `user_id` int(11) NOT NULL,
  `beat_id` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_feedback_user_idx` (`user_id`),
  KEY `fk_feedback_beat_idx` (`beat_id`),
  CONSTRAINT `fk_feedback_beat` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=4990 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.groups
CREATE TABLE IF NOT EXISTS `groups` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `title` varchar(128) NOT NULL,
  `description` text NOT NULL,
  `status` varchar(32) NOT NULL,
  `owner_id` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_groups_owner_id_idx` (`owner_id`),
  CONSTRAINT `fk_groups_owner_id` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=17 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.groups_invites
CREATE TABLE IF NOT EXISTS `groups_invites` (
  `user_id` int(11) NOT NULL,
  `group_id` int(11) NOT NULL,
  `id` int(11) NOT NULL AUTO_INCREMENT,
  PRIMARY KEY (`id`),
  KEY `fk_group_invite_user_id_idx` (`user_id`),
  KEY `fk_group_invite_group_id_idx` (`group_id`),
  CONSTRAINT `fk_group_invite_group_id` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_group_invite_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=19 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.groups_requests
CREATE TABLE IF NOT EXISTS `groups_requests` (
  `user_id` int(11) NOT NULL,
  `group_id` int(11) NOT NULL,
  `id` int(11) NOT NULL AUTO_INCREMENT,
  PRIMARY KEY (`id`),
  KEY `fk_group_request_user_id_idx` (`user_id`),
  KEY `fk_group_request_group_id_idx` (`group_id`),
  CONSTRAINT `fk_group_request_group_id` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_group_request_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=38 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.likes
CREATE TABLE IF NOT EXISTS `likes` (
  `user_id` int(11) NOT NULL,
  `beat_id` int(11) NOT NULL,
  `challenge_id` int(11) NOT NULL,
  KEY `fk_user_likes_idx` (`user_id`),
  KEY `fk_beat_likes_idx` (`beat_id`),
  KEY `likes_idx_user_id_beat_id` (`user_id`,`beat_id`),
  KEY `fk_challenge_likes` (`challenge_id`),
  CONSTRAINT `fk_beat_likes` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_challenge_likes` FOREIGN KEY (`challenge_id`) REFERENCES `challenges` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_likes` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.tags
CREATE TABLE IF NOT EXISTS `tags` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `tag` varchar(64) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `tag_UNIQUE` (`tag`)
) ENGINE=InnoDB AUTO_INCREMENT=244 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.users
CREATE TABLE IF NOT EXISTS `users` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `provider` varchar(64) NOT NULL,
  `provider_id` varchar(64) NOT NULL,
  `nickname` varchar(64) NOT NULL,
  `access_token` char(60) DEFAULT '',
  `expiry` datetime DEFAULT current_timestamp(),
  `patron` tinyint(4) NOT NULL,
  `flair` varchar(64) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=8245 DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.users_groups
CREATE TABLE IF NOT EXISTS `users_groups` (
  `user_id` int(11) NOT NULL,
  `group_id` int(11) NOT NULL,
  `role` varchar(32) NOT NULL,
  KEY `fk_users_groups_user_idx` (`user_id`),
  KEY `fk_users_groups_group_idx` (`group_id`),
  CONSTRAINT `fk_users_groups_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_users_groups_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Dumping structure for table beatbattle.votes
CREATE TABLE IF NOT EXISTS `votes` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `user_id` varchar(64) NOT NULL,
  `challenge_id` int(11) NOT NULL,
  `beat_id` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_customer_id_idx` (`challenge_id`),
  KEY `fk_beat_id_idx` (`beat_id`),
  CONSTRAINT `fk_beat_id` FOREIGN KEY (`beat_id`) REFERENCES `beats` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_challenge_id` FOREIGN KEY (`challenge_id`) REFERENCES `challenges` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=35237 DEFAULT CHARSET=utf8mb4;

/*!40101 SET SQL_MODE=IFNULL(@OLD_SQL_MODE, '') */;
/*!40014 SET FOREIGN_KEY_CHECKS=IF(@OLD_FOREIGN_KEY_CHECKS IS NULL, 1, @OLD_FOREIGN_KEY_CHECKS) */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;