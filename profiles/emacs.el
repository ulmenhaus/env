(defun yapf-format ()
 (interactive)
 (let ((initial-location (point)))
   (shell-command-on-region 1 (+ (buffer-size) 1) "yapf" t t)
   (goto-char initial-location)
 ))

; TODO consider putting this only in python mode
(global-set-key [?\C-x ?\C-a] 'yapf-format)

(defun add-yapf-save-hook ()
  (add-hook 'before-save-hook 'yapf-format nil t))

; Some hooks for yapf
; (add-hook 'python-mode-hook 'add-yapf-save-hook)
; (remove-hook 'before-save-hook 'yapf-format)


; Production database client

(defun show-stuck-builds()
  (interactive)
  (term-send-raw-string "select build_code, server_name, status from build_requests where now() - last_updated >= '2 hours'::INTERVAL and status > 0 and status < 10;\n"))

(defun show-active-builds()
  (interactive)
  (term-send-raw-string "select build_code, server_name, status from build_requests where status > 0 and status < 10;\n"))

(defun show-builders()
  (interactive)
  (term-send-raw-string "select region, \"group\", status, server_name, last_updated from builds_builder where status in (0, 1) order by last_updated;\n"))

(defun db-client ()
  (interactive)
  (term "bash")
  (rename-buffer "production-db")
  (term-send-raw-string "psql $(pass dev/teams/highland/legacy/production/db_url)\n")
  (define-key term-raw-map "\C-f" 'show-stuck-builds)
  (define-key term-raw-map "\C-u" 'show-builders)
  (define-key term-raw-map "\C-b" 'show-active-builds)
  )


; TODO write docstring folder

(defun term-in-split-window()
  (interactive)
  (split-window-right)
  (other-window 1)
  (term "bash")
  (term-send-raw-string ". ~/.profile\n")
  )

(global-set-key [?\C-t] 'term-in-split-window)
(electric-indent-mode -1)

(when (>= emacs-major-version 24)
  (require 'package)
  (add-to-list
   'package-archives
   '("melpa" . "http://melpa.org/packages/")
   t)
  (package-initialize))


(let ((default-directory
	(expand-file-name "site-packages" user-emacs-directory))
      (local-pkgs nil))
  (dolist (file (directory-files default-directory))
    (and (file-directory-p file)
	 (string-match "\\`[[:alnum:]]" file)
	 (setq local-pkgs (cons file local-pkgs))))
  (normal-top-level-add-to-load-path local-pkgs))

(setq jiralib-url "https://docker.atlassian.net/") 
(require 'org-jira) 
(require 'erc)



(eval-after-load 'erc-track
  '(progn
     (defun erc-bar-move-back (n)
       "Moves back n message lines. Ignores wrapping, and server messages."
       (interactive "nHow many lines ? ")
       (re-search-backward "^.*<.*>" nil t n))

     (defun erc-bar-update-overlay ()
       "Update the overlay for current buffer, based on the content of
erc-modified-channels-alist. Should be executed on window change."
       (interactive)
       (let* ((info (assq (current-buffer) erc-modified-channels-alist))
	            (count (cadr info)))
	  (if (and info (> count erc-bar-threshold))
	           (save-excursion
		            (end-of-buffer)
			           (when (erc-bar-move-back count)
				      (let ((inhibit-field-text-motion t))
					   (move-overlay erc-bar-overlay
							  (line-beginning-position)
							   (line-end-position)
							    (current-buffer)))))
	       (delete-overlay erc-bar-overlay))))

     (defvar erc-bar-threshold 1
       "Display bar when there are more than erc-bar-threshold unread messages.")
     (defvar erc-bar-overlay nil
       "Overlay used to set bar")
     (setq erc-bar-overlay (make-overlay 0 0))
     (overlay-put erc-bar-overlay 'face '(:underline "black"))
     ;;put the hook before erc-modified-channels-update
     (defadvice erc-track-mode (after erc-bar-setup-hook
				            (&rest args) activate)
       ;;remove and add, so we know it's in the first place
       (remove-hook 'window-configuration-change-hook 'erc-bar-update-overlay)
       (add-hook 'window-configuration-change-hook 'erc-bar-update-overlay))
     (add-hook 'erc-send-completed-hook (lambda (str)
					    (erc-bar-update-overlay)))))

