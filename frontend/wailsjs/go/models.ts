export namespace main {
	
	export class AppState {
	    state: string;
	    recordingTime: number;
	    lastTranscript: string;
	    error: string;
	    currentModel: string;
	    currentProvider: string;
	    modelReady: boolean;
	    hotkeyEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.recordingTime = source["recordingTime"];
	        this.lastTranscript = source["lastTranscript"];
	        this.error = source["error"];
	        this.currentModel = source["currentModel"];
	        this.currentProvider = source["currentProvider"];
	        this.modelReady = source["modelReady"];
	        this.hotkeyEnabled = source["hotkeyEnabled"];
	    }
	}
	export class AudioInputDevice {
	    name: string;
	    isDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AudioInputDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isDefault = source["isDefault"];
	    }
	}
	export class HistoryItem {
	    id: string;
	    text: string;
	    timestamp: string;
	    duration: number;
	    audioPath?: string;
	    hasAudio: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.timestamp = source["timestamp"];
	        this.duration = source["duration"];
	        this.audioPath = source["audioPath"];
	        this.hasAudio = source["hasAudio"];
	    }
	}
	export class ModelInfo {
	    name: string;
	    displayName: string;
	    size: string;
	    sizeBytes: number;
	    downloaded: boolean;
	    englishOnly: boolean;
	    provider: string;
	    speed: number;
	    accuracy: number;
	    useCase: string;
	    recommendation: string;
	    favorite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.size = source["size"];
	        this.sizeBytes = source["sizeBytes"];
	        this.downloaded = source["downloaded"];
	        this.englishOnly = source["englishOnly"];
	        this.provider = source["provider"];
	        this.speed = source["speed"];
	        this.accuracy = source["accuracy"];
	        this.useCase = source["useCase"];
	        this.recommendation = source["recommendation"];
	        this.favorite = source["favorite"];
	    }
	}
	export class UsageStats {
	    averageWPM: number;
	    wordsThisWeek: number;
	    recordingsThisWeek: number;
	    timeSavedThisWeek: number;
	    totalRecordings: number;
	    totalWords: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.averageWPM = source["averageWPM"];
	        this.wordsThisWeek = source["wordsThisWeek"];
	        this.recordingsThisWeek = source["recordingsThisWeek"];
	        this.timeSavedThisWeek = source["timeSavedThisWeek"];
	        this.totalRecordings = source["totalRecordings"];
	        this.totalWords = source["totalWords"];
	    }
	}

}

export namespace models {
	
	export class Config {
	    provider: string;
	    model: string;
	    openaiApiKey?: string;
	    audioInputDevice?: string;
	    autoPaste: boolean;
	    showNotification: boolean;
	    recordingHotkey: string;
	    cancelHotkey: string;
	    soundEnabled?: boolean;
	    onboardingCompleted: boolean;
	    favoriteModels?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.openaiApiKey = source["openaiApiKey"];
	        this.audioInputDevice = source["audioInputDevice"];
	        this.autoPaste = source["autoPaste"];
	        this.showNotification = source["showNotification"];
	        this.recordingHotkey = source["recordingHotkey"];
	        this.cancelHotkey = source["cancelHotkey"];
	        this.soundEnabled = source["soundEnabled"];
	        this.onboardingCompleted = source["onboardingCompleted"];
	        this.favoriteModels = source["favoriteModels"];
	    }
	}
	export class DownloadState {
	    model: string;
	    status: string;
	    downloaded: number;
	    total: number;
	    progress: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.status = source["status"];
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	    }
	}

}

