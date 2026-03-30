export namespace main {
	
	export class NetworkConfig {
	    adapterName: string;
	    mode: string;
	    ipAddress: string;
	    prefixLength: number;
	    subnetMask: string;
	    gateway: string;
	    primaryDns: string;
	    secondaryDns: string;
	    dnsServers: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adapterName = source["adapterName"];
	        this.mode = source["mode"];
	        this.ipAddress = source["ipAddress"];
	        this.prefixLength = source["prefixLength"];
	        this.subnetMask = source["subnetMask"];
	        this.gateway = source["gateway"];
	        this.primaryDns = source["primaryDns"];
	        this.secondaryDns = source["secondaryDns"];
	        this.dnsServers = source["dnsServers"];
	    }
	}
	export class SavedProfile {
	    id: string;
	    name: string;
	    config: NetworkConfig;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.config = this.convertValues(source["config"], NetworkConfig);
	        this.updatedAt = source["updatedAt"];
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
	export class AppState {
	    isAdmin: boolean;
	    privilegeMode: string;
	    profiles: SavedProfile[];
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isAdmin = source["isAdmin"];
	        this.privilegeMode = source["privilegeMode"];
	        this.profiles = this.convertValues(source["profiles"], SavedProfile);
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
	export class NetworkAdapter {
	    name: string;
	    description: string;
	    status: string;
	    macAddress: string;
	    index: number;
	
	    static createFrom(source: any = {}) {
	        return new NetworkAdapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.macAddress = source["macAddress"];
	        this.index = source["index"];
	    }
	}
	

}

