import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { api, REACTIONS, DEFAULT_STATUSES, type List, type Me, type Pulse, type Space, type Task, type Workflow, type ActiveFocus } from "./api"
import { AttachmentsBlock, ModalShell, StatsCard, useConfirm, FocusWidget, FocusPresence, NotesPanel, ActivityPanel, ArchivePanel, ArchivedSpacesPanel, FieldsPanel, type FieldDef } from "./extras"
import { tr, trFormal, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"
import { TimelineView } from "./timeline"
import { WorkflowEditor } from "./workflow"
import { AssigneePicker } from "./members"
import { WorkloadPanel, ImportCard } from "./functional"
import { IconStar, IconRefresh, IconLock, IconX, IconUser, IconPause, IconSlash, IconClock, IconGrid, IconArrowLeft, IconList, IconFileText, IconActivity, IconMenu, IconColumns, IconTable, IconCheckCircle, IconMessage, IconPin, IconAlertCircle, IconArchive, IconCalendar, IconSliders, IconBarChart, IconEdit, IconCopy } from "./icons"
import { endOfDayISO, dueClass, dueLabel, formatSystemComment, StatusChip, TaskRow } from "./taskui"

function SpaceRenameAndTools() { return null }
