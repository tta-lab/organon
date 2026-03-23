// Animal interface for polymorphic behavior.
interface Animal {
    String speak();
}

// Dog implements Animal.
class Dog implements Animal {
    private String name;

    Dog(String name) {
        this.name = name;
    }

    public String speak() {
        return "Woof!";
    }
}

enum Direction {
    NORTH, SOUTH, EAST, WEST
}
