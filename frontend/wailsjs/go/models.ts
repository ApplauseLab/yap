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
	    downloaded: boolean;
	    englishOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.size = source["size"];
	        this.downloaded = source["downloaded"];
	        this.englishOnly = source["englishOnly"];
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
	    hotkeyModifiers: string[];
	    hotkeyKey: string;
	
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
	        this.hotkeyModifiers = source["hotkeyModifiers"];
	        this.hotkeyKey = source["hotkeyKey"];
	    }
	}

}

