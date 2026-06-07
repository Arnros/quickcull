export namespace bus {
	
	export class Event {
	    Type: string;
	    Payload: any;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.Payload = source["Payload"];
	    }
	}

}

export namespace domain {
	
	export class Config {
	    exiftoolPath: string;
	    debug: boolean;
	    theme: string;
	    duplicateThreshold: number;
	    autoRefresh: boolean;
	    autoRefreshSeconds: number;
	    autoAdvance: boolean;
	    startupSnapshotEnabled: boolean;
	    windowWidth: number;
	    windowHeight: number;
	    windowX: number;
	    windowY: number;
	    windowIsMaximized: boolean;
	    windowIsFullscreen: boolean;
	    shortcuts: Record<string, string>;
	    burstSeconds: number;
	    burstMaxFiles: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exiftoolPath = source["exiftoolPath"];
	        this.debug = source["debug"];
	        this.theme = source["theme"];
	        this.duplicateThreshold = source["duplicateThreshold"];
	        this.autoRefresh = source["autoRefresh"];
	        this.autoRefreshSeconds = source["autoRefreshSeconds"];
	        this.autoAdvance = source["autoAdvance"];
	        this.startupSnapshotEnabled = source["startupSnapshotEnabled"];
	        this.windowWidth = source["windowWidth"];
	        this.windowHeight = source["windowHeight"];
	        this.windowX = source["windowX"];
	        this.windowY = source["windowY"];
	        this.windowIsMaximized = source["windowIsMaximized"];
	        this.windowIsFullscreen = source["windowIsFullscreen"];
	        this.shortcuts = source["shortcuts"];
	        this.burstSeconds = source["burstSeconds"];
	        this.burstMaxFiles = source["burstMaxFiles"];
	    }
	}

}

export namespace review {
	
	export class AppStats {
	    total: number;
	    initialTotal: number;
	    trashedCount: number;
	    starredCount: number;
	    rotatedCount: number;
	    labeledCount: number;
	    undoLen: number;
	    savedPosition: number;
	    maxLabel: number;
	    heicSupported: boolean;
	    version: string;
	    ioWorkers: number;
	    hashDeferred: boolean;
	    cacheMetaGc: number;
	    cacheHashGc: number;
	    cacheDerivedGc: number;
	
	    static createFrom(source: any = {}) {
	        return new AppStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.initialTotal = source["initialTotal"];
	        this.trashedCount = source["trashedCount"];
	        this.starredCount = source["starredCount"];
	        this.rotatedCount = source["rotatedCount"];
	        this.labeledCount = source["labeledCount"];
	        this.undoLen = source["undoLen"];
	        this.savedPosition = source["savedPosition"];
	        this.maxLabel = source["maxLabel"];
	        this.heicSupported = source["heicSupported"];
	        this.version = source["version"];
	        this.ioWorkers = source["ioWorkers"];
	        this.hashDeferred = source["hashDeferred"];
	        this.cacheMetaGc = source["cacheMetaGc"];
	        this.cacheHashGc = source["cacheHashGc"];
	        this.cacheDerivedGc = source["cacheDerivedGc"];
	    }
	}
	export class ActionResponse {
	    stats: AppStats;
	    ok?: boolean;
	    index?: number;
	    total?: number;
	
	    static createFrom(source: any = {}) {
	        return new ActionResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stats = this.convertValues(source["stats"], AppStats);
	        this.ok = source["ok"];
	        this.index = source["index"];
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AnalysisProgressResponse {
	    current: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisProgressResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.total = source["total"];
	    }
	}
	export class Photo {
	    ID: string;
	    IsStarred: boolean;
	    Rotation: number;
	    Label: number;
	    IsTrashed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Photo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.IsStarred = source["IsStarred"];
	        this.Rotation = source["Rotation"];
	        this.Label = source["Label"];
	        this.IsTrashed = source["IsTrashed"];
	    }
	}
	export class AppState {
	    Root: string;
	    CacheDir: string;
	    is_partial?: boolean;
	    Photos: Record<string, Photo>;
	    VisibleOrder: string[];
	    TrashedCount: number;
	    StarredCount: number;
	    LabeledCount: number;
	    RotatedCount: number;
	    History: bus.Event[];
	    undoLen: number;
	    InitialState?: AppState;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Root = source["Root"];
	        this.CacheDir = source["CacheDir"];
	        this.is_partial = source["is_partial"];
	        this.Photos = this.convertValues(source["Photos"], Photo, true);
	        this.VisibleOrder = source["VisibleOrder"];
	        this.TrashedCount = source["TrashedCount"];
	        this.StarredCount = source["StarredCount"];
	        this.LabeledCount = source["LabeledCount"];
	        this.RotatedCount = source["RotatedCount"];
	        this.History = this.convertValues(source["History"], bus.Event);
	        this.undoLen = source["undoLen"];
	        this.InitialState = this.convertValues(source["InitialState"], AppState);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Bookmark {
	    label: string;
	    path: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new Bookmark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.path = source["path"];
	        this.icon = source["icon"];
	    }
	}
	export class BookmarkResponse {
	    bookmarks: Bookmark[];
	    home: string;
	    sep: string;
	
	    static createFrom(source: any = {}) {
	        return new BookmarkResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bookmarks = this.convertValues(source["bookmarks"], Bookmark);
	        this.home = source["home"];
	        this.sep = source["sep"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BrowseResponse {
	    path: string;
	    parent: string;
	    sep: string;
	    entries: string[];
	
	    static createFrom(source: any = {}) {
	        return new BrowseResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.parent = source["parent"];
	        this.sep = source["sep"];
	        this.entries = source["entries"];
	    }
	}
	export class BurstInfoResp {
	    count: number;
	    index: number;
	
	    static createFrom(source: any = {}) {
	        return new BurstInfoResp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.index = source["index"];
	    }
	}
	export class FileResponse {
	    filename: string;
	    type: string;
	    format: string;
	    index: number;
	    total: number;
	    folder: string;
	    starred: boolean;
	    rotation: number;
	    label: number;
	    size?: number;
	    width?: number;
	    height?: number;
	    camera?: string;
	    iso?: string;
	    aperture?: string;
	    shutter?: string;
	    focal?: string;
	    date?: string;
	    similarity?: number;
	    burst?: BurstInfoResp;
	    txID: number;
	
	    static createFrom(source: any = {}) {
	        return new FileResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.type = source["type"];
	        this.format = source["format"];
	        this.index = source["index"];
	        this.total = source["total"];
	        this.folder = source["folder"];
	        this.starred = source["starred"];
	        this.rotation = source["rotation"];
	        this.label = source["label"];
	        this.size = source["size"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.camera = source["camera"];
	        this.iso = source["iso"];
	        this.aperture = source["aperture"];
	        this.shutter = source["shutter"];
	        this.focal = source["focal"];
	        this.date = source["date"];
	        this.similarity = source["similarity"];
	        this.burst = this.convertValues(source["burst"], BurstInfoResp);
	        this.txID = source["txID"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FilterValuesResponse {
	    cameras: string[];
	    isos: string[];
	
	    static createFrom(source: any = {}) {
	        return new FilterValuesResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cameras = source["cameras"];
	        this.isos = source["isos"];
	    }
	}
	export class FilteredIndicesResponse {
	    indices: number[];
	
	    static createFrom(source: any = {}) {
	        return new FilteredIndicesResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.indices = source["indices"];
	    }
	}
	export class FolderInfo {
	    path: string;
	    count: number;
	    startIndex: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.count = source["count"];
	        this.startIndex = source["startIndex"];
	    }
	}
	export class PathResponse {
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new PathResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	
	export class RestoreResponse {
	    stats: AppStats;
	    restored: string[];
	    index: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new RestoreResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stats = this.convertValues(source["stats"], AppStats);
	        this.restored = source["restored"];
	        this.index = source["index"];
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeCapabilities {
	    rawPreview: boolean;
	    rawMetadata: boolean;
	    heicDecode: boolean;
	    exifWrite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rawPreview = source["rawPreview"];
	        this.rawMetadata = source["rawMetadata"];
	        this.heicDecode = source["heicDecode"];
	        this.exifWrite = source["exifWrite"];
	    }
	}
	export class SysCheckResponse {
	    exiftool: boolean;
	    os: string;
	    arch: string;
	    capabilities: RuntimeCapabilities;
	
	    static createFrom(source: any = {}) {
	        return new SysCheckResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exiftool = source["exiftool"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.capabilities = this.convertValues(source["capabilities"], RuntimeCapabilities);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TrashListResponse {
	    items: string[];
	
	    static createFrom(source: any = {}) {
	        return new TrashListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = source["items"];
	    }
	}
	export class UndoResponse {
	    stats: AppStats;
	    index: number;
	    total: number;
	    actionType: string;
	
	    static createFrom(source: any = {}) {
	        return new UndoResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stats = this.convertValues(source["stats"], AppStats);
	        this.index = source["index"];
	        this.total = source["total"];
	        this.actionType = source["actionType"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

